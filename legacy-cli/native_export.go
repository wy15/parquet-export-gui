package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	odpssdk "github.com/aliyun/aliyun-odps-go-sdk/odps"
	odpsdata "github.com/aliyun/aliyun-odps-go-sdk/odps/data"
	odpsdatatype "github.com/aliyun/aliyun-odps-go-sdk/odps/datatype"
	odpstableschema "github.com/aliyun/aliyun-odps-go-sdk/odps/tableschema"
	odpstunnel "github.com/aliyun/aliyun-odps-go-sdk/odps/tunnel"
	_ "github.com/aliyun/aliyun-odps-go-sdk/sqldriver"
	"github.com/apache/arrow/go/v14/arrow"
	"github.com/apache/arrow/go/v14/arrow/array"
	"github.com/apache/arrow/go/v14/arrow/memory"
	"github.com/apache/arrow/go/v14/parquet"
	"github.com/apache/arrow/go/v14/parquet/compress"
	"github.com/apache/arrow/go/v14/parquet/pqarrow"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	go_ora "github.com/sijms/go-ora/v2"
)

type columnKind int

const (
	columnString columnKind = iota
	columnInt64
	columnFloat64
	columnBool
	columnBinary
)

var (
	supportedBackends = map[string]struct{}{
		"oracle":     {},
		"mysql":      {},
		"postgresql": {},
		"maxcompute": {},
	}
	compressionCodecs = map[string]compress.Compression{
		"snappy": compress.Codecs.Snappy,
		"gzip":   compress.Codecs.Gzip,
		"brotli": compress.Codecs.Brotli,
		"lz4":    compress.Codecs.Lz4,
		"zstd":   compress.Codecs.Zstd,
		"none":   compress.Codecs.Uncompressed,
	}
)

type exportResult struct {
	OutputPath  string
	RowsWritten int
}

type columnDef struct {
	Name  string
	Field arrow.Field
	Kind  columnKind
}

type parquetExportResources struct {
	file       *os.File
	fileWriter *pqarrow.FileWriter
	builder    *array.RecordBuilder
}

func (a *App) runTask(kind string, request ExportRequest) error {
	request.Backend = strings.ToLower(strings.TrimSpace(request.Backend))

	switch kind {
	case "test":
		return a.testConnection(request)
	case "export":
		result, err := a.exportTableToParquet(request)
		if err != nil {
			return err
		}
		a.emit(TaskEvent{
			Kind:        kind,
			Type:        "success",
			OutputPath:  result.OutputPath,
			RowsWritten: result.RowsWritten,
		})
		return nil
	default:
		return fmt.Errorf("unsupported task kind: %s", kind)
	}
}

func (a *App) testConnection(request ExportRequest) error {
	if err := validateRequest(request, false, false); err != nil {
		return err
	}

	a.emit(TaskEvent{Kind: "test", Type: "log", Message: fmt.Sprintf("测试连接: %s", request.Backend)})
	db, err := openDatabase(request)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	testSQL := "SELECT 1"
	if request.Backend == "oracle" {
		testSQL = "SELECT 1 FROM DUAL"
	} else if request.Backend == "maxcompute" {
		testSQL = "SELECT 1;"
	}

	var result int
	if err := db.QueryRowContext(ctx, testSQL).Scan(&result); err != nil {
		return err
	}

	a.emit(TaskEvent{Kind: "test", Type: "log", Message: "连接成功"})
	a.emit(TaskEvent{Kind: "test", Type: "success"})
	return nil
}

func (a *App) exportTableToParquet(request ExportRequest) (exportResult, error) {
	if err := validateRequest(request, true, true); err != nil {
		return exportResult{}, err
	}

	outputPath, err := resolveOutputPath(request.OutputPath)
	if err != nil {
		return exportResult{}, err
	}
	outputPath, err = resolveExportConflict(outputPath, request.ConflictPolicy)
	if err != nil {
		return exportResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return exportResult{}, err
	}

	var result exportResult
	if request.Backend == "maxcompute" {
		result, err = a.exportMaxComputeTableToParquet(request, outputPath)
	} else {
		result, err = a.exportSQLTableToParquet(request, outputPath)
	}
	if err != nil {
		return exportResult{}, err
	}

	if err := a.rememberGeneratedFile(result.OutputPath); err != nil {
		return exportResult{}, err
	}

	return result, nil
}

func (a *App) exportSQLTableToParquet(request ExportRequest, outputPath string) (exportResult, error) {
	db, err := openDatabase(request)
	if err != nil {
		return exportResult{}, err
	}
	defer db.Close()

	query := buildExportQuery(request)
	a.emit(TaskEvent{Kind: "export", Type: "log", Message: fmt.Sprintf("准备导出 %s -> %s", fullTableName(request), outputPath)})
	a.emit(TaskEvent{Kind: "export", Type: "log", Message: fmt.Sprintf("执行查询: %s", query)})

	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		return exportResult{}, err
	}
	defer rows.Close()

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return exportResult{}, err
	}

	columnDefs := inferColumnDefs(columnTypes)
	schema := arrow.NewSchema(columnFields(columnDefs), nil)
	resources, err := newParquetExportResources(outputPath, schema, request.Compression, request.normalizedBatchSize())
	if err != nil {
		return exportResult{}, err
	}
	defer resources.close()

	values := make([]any, len(columnDefs))
	scanTargets := make([]any, len(columnDefs))
	for i := range values {
		scanTargets[i] = &values[i]
	}

	batchRows := 0
	totalRows := 0

	for rows.Next() {
		if err := rows.Scan(scanTargets...); err != nil {
			return exportResult{}, err
		}

		if err := appendRow(resources.builder, columnDefs, values); err != nil {
			return exportResult{}, err
		}

		batchRows++
		if batchRows >= request.normalizedBatchSize() {
			if err := flushRecordBatch(resources.fileWriter, resources.builder); err != nil {
				return exportResult{}, err
			}
			totalRows += batchRows
			a.emit(TaskEvent{Kind: "export", Type: "log", Message: fmt.Sprintf("已写入 %s 行", formatRows(totalRows))})
			batchRows = 0
		}
	}

	if err := rows.Err(); err != nil {
		return exportResult{}, err
	}

	if batchRows > 0 {
		if err := flushRecordBatch(resources.fileWriter, resources.builder); err != nil {
			return exportResult{}, err
		}
		totalRows += batchRows
		a.emit(TaskEvent{Kind: "export", Type: "log", Message: fmt.Sprintf("已写入 %s 行", formatRows(totalRows))})
	}

	if totalRows == 0 {
		a.emit(TaskEvent{Kind: "export", Type: "log", Message: "源表为空，已生成空的 Parquet 文件"})
	}
	a.emit(TaskEvent{Kind: "export", Type: "log", Message: fmt.Sprintf("导出完成，总计 %s 行", formatRows(totalRows))})

	if err := resources.finish(); err != nil {
		return exportResult{}, err
	}

	return exportResult{OutputPath: outputPath, RowsWritten: totalRows}, nil
}

func (a *App) exportMaxComputeTableToParquet(request ExportRequest, outputPath string) (exportResult, error) {
	session, err := openMaxComputeDownloadSession(request)
	if err != nil {
		return exportResult{}, err
	}

	columnDefs := inferMaxComputeColumnDefs(session.Schema().Columns)
	schema := arrow.NewSchema(columnFields(columnDefs), nil)
	resources, err := newParquetExportResources(outputPath, schema, request.Compression, request.normalizedBatchSize())
	if err != nil {
		return exportResult{}, err
	}
	defer resources.close()

	totalRows := 0
	recordCount := session.RecordCount()
	a.emit(TaskEvent{
		Kind:    "export",
		Type:    "log",
		Message: fmt.Sprintf("使用 MaxCompute Tunnel 批量下载 %s -> %s", fullTableName(request), outputPath),
	})
	a.emit(TaskEvent{
		Kind:    "export",
		Type:    "log",
		Message: fmt.Sprintf("创建 DownloadSession 成功，记录数: %s", formatRows(recordCount)),
	})

	for start := 0; start < recordCount; start += request.normalizedBatchSize() {
		count := request.normalizedBatchSize()
		if remaining := recordCount - start; remaining < count {
			count = remaining
		}

		reader, err := session.OpenRecordReader(start, count, nil)
		if err != nil {
			return exportResult{}, err
		}

		batchRows, err := a.writeMaxComputeBatch(reader, resources.builder, columnDefs)
		closeErr := reader.Close()
		if err != nil {
			return exportResult{}, err
		}
		if closeErr != nil {
			return exportResult{}, closeErr
		}

		if err := flushRecordBatch(resources.fileWriter, resources.builder); err != nil {
			return exportResult{}, err
		}

		totalRows += batchRows
		a.emit(TaskEvent{
			Kind:    "export",
			Type:    "log",
			Message: fmt.Sprintf("已写入 %s 行", formatRows(totalRows)),
		})
	}

	if totalRows == 0 {
		a.emit(TaskEvent{Kind: "export", Type: "log", Message: "源表为空，已生成空的 Parquet 文件"})
	}
	a.emit(TaskEvent{Kind: "export", Type: "log", Message: fmt.Sprintf("导出完成，总计 %s 行", formatRows(totalRows))})

	if err := resources.finish(); err != nil {
		return exportResult{}, err
	}

	return exportResult{OutputPath: outputPath, RowsWritten: totalRows}, nil
}

func (a *App) writeMaxComputeBatch(
	reader *odpstunnel.RecordProtocReader,
	builder *array.RecordBuilder,
	columnDefs []columnDef,
) (int, error) {
	rowsWritten := 0

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return rowsWritten, nil
		}
		if err != nil {
			return rowsWritten, err
		}

		values := make([]any, len(record))
		for idx, value := range record {
			values[idx] = convertMaxComputeValue(value)
		}

		if err := appendRow(builder, columnDefs, values); err != nil {
			return rowsWritten, err
		}
		rowsWritten++
	}
}

func validateRequest(request ExportRequest, requireOutput bool, requireTable bool) error {
	if _, ok := supportedBackends[request.Backend]; !ok {
		return fmt.Errorf("不支持的数据源: %s", request.Backend)
	}
	if requireTable && strings.TrimSpace(request.Table) == "" {
		return errors.New("表名不能为空")
	}
	if request.normalizedBatchSize() <= 0 {
		return errors.New("批次大小必须大于 0")
	}
	if requireOutput && strings.TrimSpace(request.OutputPath) == "" {
		return errors.New("输出路径不能为空")
	}
	if _, ok := compressionCodecs[request.Compression]; !ok {
		return errors.New("不支持的压缩格式")
	}
	missing := []string{}
	if request.Backend == "maxcompute" {
		if strings.TrimSpace(request.MaxComputeEndpoint) == "" {
			missing = append(missing, "MaxCompute Endpoint")
		}
		if strings.TrimSpace(request.MaxComputeProject) == "" {
			missing = append(missing, "MaxCompute Project")
		}
		if strings.TrimSpace(request.MaxComputeAccessID) == "" {
			missing = append(missing, "Access ID")
		}
		if strings.TrimSpace(request.MaxComputeAccessKey) == "" {
			missing = append(missing, "Access Key")
		}
	} else {
		if strings.TrimSpace(request.Host) == "" {
			missing = append(missing, "Host")
		}
		if strings.TrimSpace(request.Username) == "" {
			missing = append(missing, "Username")
		}
		if strings.TrimSpace(request.Database) == "" {
			missing = append(missing, "Database / Service Name")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("缺少必填项: %s", strings.Join(missing, ", "))
	}
	return nil
}

func openDatabase(request ExportRequest) (*sql.DB, error) {
	var (
		driverName string
		dsn        string
	)

	switch request.Backend {
	case "oracle":
		driverName = "oracle"
		dsn = go_ora.BuildUrl(
			request.Host,
			request.portOrDefault(),
			request.Database,
			request.Username,
			request.Password,
			nil,
		)
	case "mysql":
		driverName = "mysql"
		dsn = mysqlDSN(request)
	case "postgresql":
		driverName = "pgx"
		dsn = postgresDSN(request)
	case "maxcompute":
		driverName = "odps"
		dsn = maxComputeDSN(request)
	default:
		return nil, fmt.Errorf("不支持的数据源: %s", request.Backend)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	return db, nil
}

func mysqlDSN(request ExportRequest) string {
	query := url.Values{}
	query.Set("charset", "utf8mb4")
	query.Set("parseTime", "true")
	query.Set("loc", "Local")
	return fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?%s",
		request.Username,
		request.Password,
		net.JoinHostPort(request.Host, strconv.Itoa(request.portOrDefault())),
		request.Database,
		query.Encode(),
	)
}

func postgresDSN(request ExportRequest) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		request.Host,
		request.portOrDefault(),
		request.Username,
		request.Password,
		request.Database,
	)
}

func maxComputeDSN(request ExportRequest) string {
	endpoint := strings.TrimSpace(request.MaxComputeEndpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}

	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint
	}

	u.User = url.UserPassword(
		strings.TrimSpace(request.MaxComputeAccessID),
		strings.TrimSpace(request.MaxComputeAccessKey),
	)

	query := u.Query()
	query.Set("project", strings.TrimSpace(request.MaxComputeProject))
	query.Set("odps.sql.type.system.odps2", "true")
	query.Set("odps.sql.decimal.odps2", "true")
	u.RawQuery = query.Encode()

	return u.String()
}

func inferColumnDefs(columnTypes []*sql.ColumnType) []columnDef {
	result := make([]columnDef, 0, len(columnTypes))
	for _, columnType := range columnTypes {
		kind, dataType := inferColumnType(columnType)
		result = append(result, columnDef{
			Name: columnType.Name(),
			Kind: kind,
			Field: arrow.Field{
				Name:     columnType.Name(),
				Type:     dataType,
				Nullable: true,
			},
		})
	}
	return result
}

func inferColumnType(columnType *sql.ColumnType) (columnKind, arrow.DataType) {
	dbType := strings.ToUpper(columnType.DatabaseTypeName())
	precision, scale, hasDecimal := columnType.DecimalSize()

	switch {
	case strings.Contains(dbType, "BOOL"):
		return columnBool, arrow.FixedWidthTypes.Boolean
	case strings.Contains(dbType, "BLOB"),
		strings.Contains(dbType, "BINARY"),
		strings.Contains(dbType, "RAW"),
		strings.Contains(dbType, "BYTEA"):
		return columnBinary, arrow.BinaryTypes.Binary
	case strings.Contains(dbType, "INT"),
		strings.Contains(dbType, "SERIAL"),
		strings.Contains(dbType, "SMALLINT"),
		strings.Contains(dbType, "BIGINT"):
		return columnInt64, arrow.PrimitiveTypes.Int64
	case strings.Contains(dbType, "FLOAT"),
		strings.Contains(dbType, "DOUBLE"),
		strings.Contains(dbType, "REAL"):
		return columnFloat64, arrow.PrimitiveTypes.Float64
	case hasDecimal && scale == 0 && precision <= 18:
		return columnInt64, arrow.PrimitiveTypes.Int64
	case hasDecimal:
		return columnFloat64, arrow.PrimitiveTypes.Float64
	default:
		return columnString, arrow.BinaryTypes.String
	}
}

func appendRow(builder *array.RecordBuilder, columnDefs []columnDef, values []any) error {
	for idx, column := range columnDefs {
		value := values[idx]
		switch column.Kind {
		case columnString:
			if value == nil {
				builder.Field(idx).AppendNull()
				continue
			}
			builder.Field(idx).(*array.StringBuilder).Append(toString(value))
		case columnInt64:
			if value == nil {
				builder.Field(idx).AppendNull()
				continue
			}
			number, ok := toInt64(value)
			if !ok {
				builder.Field(idx).AppendNull()
				continue
			}
			builder.Field(idx).(*array.Int64Builder).Append(number)
		case columnFloat64:
			if value == nil {
				builder.Field(idx).AppendNull()
				continue
			}
			number, ok := toFloat64(value)
			if !ok {
				builder.Field(idx).AppendNull()
				continue
			}
			builder.Field(idx).(*array.Float64Builder).Append(number)
		case columnBool:
			if value == nil {
				builder.Field(idx).AppendNull()
				continue
			}
			boolean, ok := toBool(value)
			if !ok {
				builder.Field(idx).AppendNull()
				continue
			}
			builder.Field(idx).(*array.BooleanBuilder).Append(boolean)
		case columnBinary:
			if value == nil {
				builder.Field(idx).AppendNull()
				continue
			}
			builder.Field(idx).(*array.BinaryBuilder).Append(toBytes(value))
		default:
			return fmt.Errorf("unsupported column kind: %d", column.Kind)
		}
	}
	return nil
}

func flushRecordBatch(writer *pqarrow.FileWriter, builder *array.RecordBuilder) error {
	record := builder.NewRecord()
	defer record.Release()
	if record.NumRows() == 0 {
		return nil
	}
	return writer.WriteBuffered(record)
}

func newParquetExportResources(
	outputPath string,
	schema *arrow.Schema,
	compression string,
	batchSize int,
) (*parquetExportResources, error) {
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}

	writerProps := parquet.NewWriterProperties(
		parquet.WithCompression(parquetCompression(compression)),
		parquet.WithDictionaryDefault(false),
	)
	fileWriter, err := pqarrow.NewFileWriter(
		schema,
		file,
		writerProps,
		pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema()),
	)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	builder.Reserve(batchSize)

	return &parquetExportResources{
		file:       file,
		fileWriter: fileWriter,
		builder:    builder,
	}, nil
}

func (r *parquetExportResources) finish() error {
	if r == nil || r.fileWriter == nil {
		return nil
	}

	if err := r.fileWriter.Close(); err != nil {
		return err
	}
	r.fileWriter = nil

	if r.file != nil {
		r.file = nil
	}
	return nil
}

func (r *parquetExportResources) close() {
	if r == nil {
		return
	}
	if r.builder != nil {
		r.builder.Release()
		r.builder = nil
	}
	if r.fileWriter != nil {
		_ = r.fileWriter.Close()
		r.fileWriter = nil
	}
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
}

func parquetCompression(name string) compress.Compression {
	codec, ok := compressionCodecs[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return compress.Codecs.Snappy
	}
	return codec
}

func columnFields(defs []columnDef) []arrow.Field {
	fields := make([]arrow.Field, 0, len(defs))
	for _, def := range defs {
		fields = append(fields, def.Field)
	}
	return fields
}

func resolveOutputPath(path string) (string, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return "", errors.New("输出路径不能为空")
	}
	if strings.HasPrefix(cleaned, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cleaned = filepath.Join(home, strings.TrimPrefix(cleaned, "~/"))
	}
	return filepath.Abs(filepath.Clean(cleaned))
}

func resolveExportConflict(outputPath, policy string) (string, error) {
	exists, err := fileExists(outputPath)
	if err != nil {
		return "", err
	}
	if !exists {
		return outputPath, nil
	}

	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "overwrite":
		return outputPath, nil
	case "rename":
		return nextAvailablePath(outputPath)
	default:
		return "", fmt.Errorf("不支持的文件冲突策略: %s", policy)
	}
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func nextAvailablePath(path string) (string, error) {
	extension := filepath.Ext(path)
	base := strings.TrimSuffix(path, extension)
	candidate := path

	for index := 1; ; index++ {
		exists, err := fileExists(candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d%s", base, index, extension)
	}
}

func openMaxComputeDownloadSession(request ExportRequest) (*odpstunnel.DownloadSession, error) {
	cfg := odpssdk.NewConfig()
	cfg.AccessId = strings.TrimSpace(request.MaxComputeAccessID)
	cfg.AccessKey = strings.TrimSpace(request.MaxComputeAccessKey)
	cfg.Endpoint = strings.TrimSpace(request.MaxComputeEndpoint)
	cfg.ProjectName = strings.TrimSpace(request.MaxComputeProject)

	odpsIns := cfg.GenOdps()
	schemaName := strings.TrimSpace(request.Schema)

	tunnelIns := odpstunnel.NewTunnel(odpsIns)
	options := make([]odpstunnel.Option, 0, 2)
	if schemaName != "" {
		options = append(options, odpstunnel.SessionCfg.WithSchemaName(schemaName))
	}
	if partitionSpec := strings.TrimSpace(request.PartitionSpec); partitionSpec != "" {
		options = append(options, odpstunnel.SessionCfg.WithPartitionKey(partitionSpec))
	}

	return tunnelIns.CreateDownloadSession(
		strings.TrimSpace(request.MaxComputeProject),
		strings.TrimSpace(request.Table),
		options...,
	)
}

func fullTableName(request ExportRequest) string {
	if request.Backend == "maxcompute" {
		parts := make([]string, 0, 3)
		if project := strings.TrimSpace(request.MaxComputeProject); project != "" {
			parts = append(parts, project)
		}
		if schema := strings.TrimSpace(request.Schema); schema != "" {
			parts = append(parts, schema)
		}
		parts = append(parts, strings.TrimSpace(request.Table))

		name := strings.Join(parts, ".")
		if partition := strings.TrimSpace(request.PartitionSpec); partition != "" {
			name += " PARTITION (" + partition + ")"
		}
		return name
	}

	if strings.TrimSpace(request.Schema) == "" {
		return strings.TrimSpace(request.Table)
	}
	return strings.TrimSpace(request.Schema) + "." + strings.TrimSpace(request.Table)
}

func buildExportQuery(request ExportRequest) string {
	query := "SELECT * FROM " + fullTableName(request)
	if request.Backend == "maxcompute" {
		return query + ";"
	}
	return query
}

func inferMaxComputeColumnDefs(columns []odpstableschema.Column) []columnDef {
	result := make([]columnDef, 0, len(columns))
	for _, column := range columns {
		kind, dataType := inferMaxComputeColumnType(column.Type)
		result = append(result, columnDef{
			Name: column.Name,
			Kind: kind,
			Field: arrow.Field{
				Name:     column.Name,
				Type:     dataType,
				Nullable: true,
			},
		})
	}
	return result
}

func inferMaxComputeColumnType(dataType odpsdatatype.DataType) (columnKind, arrow.DataType) {
	switch dataType.ID() {
	case odpsdatatype.BOOLEAN:
		return columnBool, arrow.FixedWidthTypes.Boolean
	case odpsdatatype.BIGINT, odpsdatatype.INT, odpsdatatype.SMALLINT, odpsdatatype.TINYINT:
		return columnInt64, arrow.PrimitiveTypes.Int64
	case odpsdatatype.FLOAT, odpsdatatype.DOUBLE:
		return columnFloat64, arrow.PrimitiveTypes.Float64
	case odpsdatatype.BINARY:
		return columnBinary, arrow.BinaryTypes.Binary
	default:
		return columnString, arrow.BinaryTypes.String
	}
}

func convertMaxComputeValue(value odpsdata.Data) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case odpsdata.Bool:
		return bool(typed)
	case odpsdata.BigInt:
		return int64(typed)
	case odpsdata.Int:
		return int64(typed)
	case odpsdata.SmallInt:
		return int64(typed)
	case odpsdata.TinyInt:
		return int64(typed)
	case odpsdata.Float:
		return float64(typed)
	case odpsdata.Double:
		return float64(typed)
	case odpsdata.String:
		return string(typed)
	case odpsdata.Binary:
		return []byte(typed)
	case odpsdata.Date:
		return typed.String()
	case odpsdata.DateTime:
		return typed.String()
	case odpsdata.Timestamp:
		return typed.String()
	case odpsdata.Char:
		return typed.String()
	case odpsdata.VarChar:
		return typed.String()
	case *odpsdata.Decimal:
		return typed.String()
	case *odpsdata.Json:
		return typed.String()
	case *odpsdata.Array:
		return typed.String()
	case *odpsdata.Map:
		return typed.String()
	case *odpsdata.Struct:
		return typed.String()
	case odpsdata.IntervalDayTime:
		return typed.String()
	case odpsdata.IntervalYearMonth:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func formatRows(value int) string {
	text := strconv.Itoa(value)
	if len(text) <= 3 {
		return text
	}
	var parts []string
	for len(text) > 3 {
		parts = append([]string{text[len(text)-3:]}, parts...)
		text = text[:len(text)-3]
	}
	if text != "" {
		parts = append([]string{text}, parts...)
	}
	return strings.Join(parts, ",")
}

func toString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		if utf8.Valid(typed) {
			return string(typed)
		}
		return fmt.Sprintf("%x", typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(value)
	}
}

func toBytes(value any) []byte {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return append([]byte(nil), typed...)
	case string:
		return []byte(typed)
	default:
		return []byte(fmt.Sprint(value))
	}
}

func toInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		if uint64(typed) > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		if math.Trunc(float64(typed)) != float64(typed) {
			return 0, false
		}
		return int64(typed), true
	case float64:
		if math.Trunc(typed) != typed {
			return 0, false
		}
		return int64(typed), true
	case []byte:
		return parseInt64(string(typed))
	case string:
		return parseInt64(typed)
	default:
		return 0, false
	}
}

func parseInt64(value string) (int64, bool) {
	number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return number, err == nil
}

func toFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case []byte:
		return parseFloat64(string(typed))
	case string:
		return parseFloat64(typed)
	default:
		return 0, false
	}
}

func parseFloat64(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return number, err == nil
}

func toBool(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case int:
		return typed != 0, true
	case int8:
		return typed != 0, true
	case int16:
		return typed != 0, true
	case int32:
		return typed != 0, true
	case int64:
		return typed != 0, true
	case uint:
		return typed != 0, true
	case uint8:
		return typed != 0, true
	case uint16:
		return typed != 0, true
	case uint32:
		return typed != 0, true
	case uint64:
		return typed != 0, true
	case []byte:
		return parseBool(string(typed))
	case string:
		return parseBool(typed)
	default:
		return false, false
	}
}

func parseBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y":
		return true, true
	case "0", "false", "f", "no", "n":
		return false, true
	default:
		return false, false
	}
}

func (r ExportRequest) portOrDefault() int {
	if r.Port > 0 {
		return r.Port
	}

	switch strings.ToLower(strings.TrimSpace(r.Backend)) {
	case "oracle":
		return 1521
	case "mysql":
		return 3306
	case "postgresql":
		return 5432
	case "maxcompute":
		return 443
	default:
		return 0
	}
}

func (r ExportRequest) normalizedBatchSize() int {
	if r.BatchSize > 0 {
		return r.BatchSize
	}
	return 5000
}
