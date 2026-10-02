"""Draw the AntennaDeck icon: dark compass, green beam wedge, orange north marker."""
import math
import os
import sys

from PIL import Image, ImageDraw, ImageFont

out_dir = sys.argv[1]
S = 1024 * 2  # draw at 2x and downsample for smooth edges


def lerp(a, b, t):
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(len(a)))


img = Image.new("RGBA", (S, S), (0, 0, 0, 0))

# rounded-square background with a vertical gradient
bg = Image.new("RGBA", (S, S))
bd = ImageDraw.Draw(bg)
for y in range(S):
    bd.line([(0, y), (S, y)], fill=lerp((46, 39, 33, 255), (16, 14, 12, 255), y / S))
mask = Image.new("L", (S, S), 0)
ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=int(S * 0.2), fill=255)
img.paste(bg, (0, 0), mask)

d = ImageDraw.Draw(img)
cx, cy, R = S // 2, int(S * 0.53), int(S * 0.36)

# compass face: radial gradient
for i in range(R, 0, -4):
    t = 1 - i / R
    d.ellipse([cx - i, cy - i, cx + i, cy + i], fill=lerp((11, 15, 19, 255), (58, 74, 88, 255), t ** 1.6))
d.ellipse([cx - R, cy - R, cx + R, cy + R], outline=(110, 116, 124, 255), width=int(S * 0.018))

# tick marks every 30 degrees
for deg in range(0, 360, 30):
    a = math.radians(deg - 90)
    r1, r2 = R * (0.78 if deg % 90 == 0 else 0.86), R * 0.97
    d.line([(cx + r1 * math.cos(a), cy + r1 * math.sin(a)), (cx + r2 * math.cos(a), cy + r2 * math.sin(a))],
           fill=(225, 225, 225, 255), width=int(S * (0.016 if deg % 90 == 0 else 0.009)))

# beam wedge toward ~40 degrees
heading, width = 40, 54
wedge = Image.new("RGBA", (S, S), (0, 0, 0, 0))
wd = ImageDraw.Draw(wedge)
box = [cx - R * 0.9, cy - R * 0.9, cx + R * 0.9, cy + R * 0.9]
wd.pieslice(box, heading - width / 2 - 90, heading + width / 2 - 90, fill=(34, 194, 122, 120),
            outline=(60, 230, 150, 255), width=int(S * 0.008))
img = Image.alpha_composite(img, wedge)
d = ImageDraw.Draw(img)

# needle
a = math.radians(heading - 90)
d.line([(cx, cy), (cx + R * 0.9 * math.cos(a), cy + R * 0.9 * math.sin(a))], fill=(34, 214, 132, 255),
       width=int(S * 0.03))

# hub
h = int(S * 0.035)
d.ellipse([cx - h, cy - h, cx + h, cy + h], fill=(232, 118, 43, 255))

# orange north marker above the ring
top = cy - R - int(S * 0.03)
tw = int(S * 0.075)
d.polygon([(cx, top - int(S * 0.11)), (cx - tw, top), (cx + tw, top)], fill=(232, 118, 43, 255))

icon = img.resize((1024, 1024), Image.LANCZOS)
os.makedirs(os.path.join(out_dir, "windows"), exist_ok=True)
icon.save(os.path.join(out_dir, "appicon.png"))
icon.save(os.path.join(out_dir, "windows", "icon.ico"),
          sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
print("ok")
