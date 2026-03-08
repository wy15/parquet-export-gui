#!/usr/bin/env python3

from __future__ import annotations

from pathlib import Path
from PIL import Image, ImageDraw, ImageFilter
import math


ROOT = Path(__file__).resolve().parent.parent
BUILD_DIR = ROOT / "build"
DARWIN_DIR = BUILD_DIR / "darwin"
WINDOWS_DIR = BUILD_DIR / "windows"

SIZE = 1024
RADIUS = 228


def lerp(a: int, b: int, t: float) -> int:
    return round(a + (b - a) * t)


def blend(c1: tuple[int, int, int], c2: tuple[int, int, int], t: float) -> tuple[int, int, int]:
    return tuple(lerp(c1[i], c2[i], t) for i in range(3))


def gradient_background(size: int) -> Image.Image:
    top = (18, 36, 61)
    bottom = (39, 118, 114)
    image = Image.new("RGBA", (size, size))
    pixels = image.load()
    center = (size * 0.78, size * 0.24)
    radius = size * 0.92

    for y in range(size):
        for x in range(size):
            vertical = y / (size - 1)
            radial = max(0.0, 1.0 - math.dist((x, y), center) / radius)
            color = blend(top, bottom, min(1.0, vertical * 0.9 + radial * 0.28))
            pixels[x, y] = (*color, 255)

    return image


def rounded_mask(size: int, radius: int) -> Image.Image:
    mask = Image.new("L", (size, size), 0)
    draw = ImageDraw.Draw(mask)
    draw.rounded_rectangle((32, 32, size - 32, size - 32), radius=radius, fill=255)
    return mask


def draw_columns(base: Image.Image) -> None:
    draw = ImageDraw.Draw(base)
    column_fill = (246, 250, 252, 255)
    column_shadow = Image.new("RGBA", base.size, (0, 0, 0, 0))
    shadow_draw = ImageDraw.Draw(column_shadow)

    columns = [
        (220, 202, 360, 784),
        (442, 202, 582, 690),
        (664, 202, 804, 596),
    ]

    for rect in columns:
        shadow_draw.rounded_rectangle(
            (rect[0] + 10, rect[1] + 18, rect[2] + 10, rect[3] + 18),
            radius=44,
            fill=(7, 14, 28, 62),
        )

    shadow = column_shadow.filter(ImageFilter.GaussianBlur(22))
    base.alpha_composite(shadow)

    for rect in columns:
        draw.rounded_rectangle(rect, radius=44, fill=column_fill)

    separators = [
        (220, 328, 804, 344),
        (220, 472, 804, 488),
        (220, 616, 582, 632),
    ]
    for rect in separators:
        draw.rounded_rectangle(rect, radius=8, fill=(207, 224, 230, 255))


def draw_arrow(base: Image.Image) -> None:
    glow = Image.new("RGBA", base.size, (0, 0, 0, 0))
    glow_draw = ImageDraw.Draw(glow)
    glow_draw.rounded_rectangle((334, 642, 706, 736), radius=46, fill=(255, 116, 70, 110))
    glow_draw.polygon(((638, 574), (858, 690), (638, 804)), fill=(255, 116, 70, 118))
    base.alpha_composite(glow.filter(ImageFilter.GaussianBlur(30)))

    draw = ImageDraw.Draw(base)
    draw.rounded_rectangle((322, 652, 676, 726), radius=40, fill=(245, 96, 61, 255))
    draw.polygon(((612, 556), (858, 690), (612, 824)), fill=(245, 96, 61, 255))

    draw.rounded_rectangle((332, 662, 666, 675), radius=6, fill=(255, 181, 154, 140))
    draw.polygon(((630, 590), (808, 690), (630, 790)), fill=(255, 181, 154, 84))


def draw_border(base: Image.Image) -> None:
    overlay = Image.new("RGBA", base.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    draw.rounded_rectangle(
        (32, 32, SIZE - 32, SIZE - 32),
        radius=RADIUS,
        outline=(255, 255, 255, 58),
        width=4,
    )
    draw.rounded_rectangle(
        (60, 60, SIZE - 60, SIZE - 60),
        radius=RADIUS - 28,
        outline=(255, 255, 255, 24),
        width=2,
    )
    base.alpha_composite(overlay)


def build_icon() -> Image.Image:
    icon = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))

    shadow = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    shadow_draw = ImageDraw.Draw(shadow)
    shadow_draw.rounded_rectangle((42, 54, SIZE - 42, SIZE - 10), radius=RADIUS, fill=(0, 0, 0, 86))
    shadow = shadow.filter(ImageFilter.GaussianBlur(32))
    icon.alpha_composite(shadow)

    plate = gradient_background(SIZE)
    plate.putalpha(rounded_mask(SIZE, RADIUS))
    icon.alpha_composite(plate)

    haze = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    haze_draw = ImageDraw.Draw(haze)
    haze_draw.ellipse((122, 96, 650, 522), fill=(108, 215, 208, 36))
    haze_draw.ellipse((514, 510, 932, 930), fill=(255, 142, 109, 24))
    icon.alpha_composite(haze.filter(ImageFilter.GaussianBlur(42)))

    draw_columns(icon)
    draw_arrow(icon)
    draw_border(icon)
    return icon


def save_pngs(icon: Image.Image) -> None:
    BUILD_DIR.mkdir(parents=True, exist_ok=True)
    WINDOWS_DIR.mkdir(parents=True, exist_ok=True)
    DARWIN_DIR.mkdir(parents=True, exist_ok=True)

    png_path = BUILD_DIR / "appicon.png"
    icon.save(png_path)

    ico_path = WINDOWS_DIR / "icon.ico"
    icon.save(
        ico_path,
        sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)],
    )


def save_icns(icon: Image.Image) -> None:
    target = DARWIN_DIR / "iconfile.icns"
    icon.save(target)


def main() -> None:
    icon = build_icon()
    save_pngs(icon)
    save_icns(icon)


if __name__ == "__main__":
    main()
