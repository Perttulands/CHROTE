import struct
import math
import os

def create_ico(file_path):
    sizes = [256, 128, 64, 48, 32, 16]
    images_data = []
    
    # 1. Generate image data for each size
    for size in sizes:
        print(f"Generating size {size}x{size}...")
        pixels = generate_pixels(size)
        bmp_data = create_bmp_bytes(size, pixels)
        images_data.append(bmp_data)

    # 2. Write ICO file
    with open(file_path, 'wb') as f:
        # Header: Reserved (2), Type (2=ICO), Count (2)
        f.write(struct.pack('<HHH', 0, 1, len(sizes)))
        
        offset = 6 + (16 * len(sizes))
        
        # Directory entries
        for i, size in enumerate(sizes):
            width = 0 if size == 256 else size
            height = 0 if size == 256 else size
            colors = 0
            reserved = 0
            planes = 1
            bpp = 32
            data_size = len(images_data[i])
            
            f.write(struct.pack('<BBBBHHII', 
                width, height, colors, reserved, 
                planes, bpp, data_size, offset))
            
            offset += data_size
            
        # Image data
        for data in images_data:
            f.write(data)
            
    print(f"Icon saved to {file_path}")

def generate_pixels(size):
    # Returns raw BGRA data (top-left to bottom-right, but BMP expects bottom-up, 
    # so we will generate normally and flip when packing or just calc inverted Y)
    # Actually, standard DIBs in ICO are often stored bottom-up. 
    # Let's calculate colors for grid (x, y) where (0,0) is bottom-left.
    
    buffer = bytearray(size * size * 4)
    
    # Colors (R, G, B, A)
    col_bg = (15, 15, 25, 255)       # Deep Purple/Blue
    col_primary = (0, 240, 255, 255) # Cyan/Teal
    col_accent = (180, 40, 255, 255) # Purple/Magenta
    col_white = (220, 240, 255, 255) # Whiteish
    
    # Geometry Helper
    def get_color(x, y):
        # Normalized coords 0.0 to 1.0
        nx = x / size
        ny = y / size # 0 is bottom, 1 is top
        
        # Background - circular gradient vignette
        # dist from center
        dx = nx - 0.5
        dy = ny - 0.5
        d = math.sqrt(dx*dx + dy*dy)
        if d > 0.5:
            # Darken corners
            factor = max(0, 1.0 - (d - 0.5) * 2)
            bg = (int(col_bg[0]*factor), int(col_bg[1]*factor), int(col_bg[2]*factor), 255)
        else:
            bg = col_bg
            
        final_color = bg
        
        # Anti-aliased line drawing helper
        def line_dist(px, py, x1, y1, x2, y2):
            # Distance from point (px,py) to line segment (x1,y1)-(x2,y2)
            l2 = (x1-x2)**2 + (y1-y2)**2
            if l2 == 0: return math.hypot(px-x1, py-y1)
            t = ((px-x1)*(x2-x1) + (py-y1)*(y2-y1)) / l2
            t = max(0, min(1, t))
            return math.hypot(px - (x1 + t*(x2-x1)), py - (y1 + t*(y2-y1)))

        thickness = 0.08
        if size < 32: thickness = 0.12 # thicker for small icons
        
        # --- SHAPES ---
        
        alpha = 0.0
        
        # 1. The "A" legs
        # Top point (0.5, 0.85)
        # Left Leg Base (0.2, 0.2)
        # Right Leg Base (0.8, 0.2)
        
        apex = (0.5, 0.85)
        left_base = (0.2, 0.2)
        right_base = (0.8, 0.2)
        
        d_left = line_dist(nx, ny, left_base[0], left_base[1], apex[0], apex[1])
        d_right = line_dist(nx, ny, right_base[0], right_base[1], apex[0], apex[1])
        
        # 2. Horizontal Bar
        # (0.35, 0.45) to (0.65, 0.45)
        bar_y = 0.45
        d_bar = 1.0
        if 0.3 <= nx <= 0.7:
           d_bar = abs(ny - bar_y)
           
        dist = min(d_left, d_right, d_bar if 0.25 < nx < 0.75 else 1.0)
        
        if dist < thickness / 2:
            alpha = 1.0
        elif dist < (thickness / 2) + 0.02:
            # simple AA
            alpha = 1.0 - (dist - thickness/2) / 0.02
            
        if alpha > 0:
            # Blend
            r = int(col_primary[0] * alpha + final_color[0] * (1-alpha))
            g = int(col_primary[1] * alpha + final_color[1] * (1-alpha))
            b = int(col_primary[2] * alpha + final_color[2] * (1-alpha))
            final_color = (r,g,b, 255)
            
        # 3. Circuit Dots / Terminals (at feet)
        for px, py in [left_base, right_base, apex]:
            dd = math.sqrt((nx-px)**2 + (ny-py)**2)
            rad = 0.06
            if dd < rad:
                a2 = 1.0
                if dd > rad - 0.02:
                     a2 = 1.0 - (dd - (rad-0.02))/0.02
                
                # Use Accent Color for dots
                r = int(col_accent[0] * a2 + final_color[0] * (1-a2))
                g = int(col_accent[1] * a2 + final_color[1] * (1-a2))
                b = int(col_accent[2] * a2 + final_color[2] * (1-a2))
                final_color = (r,g,b, 255)

        # 4. Underscore Cursor
        # (0.4, 0.1) to (0.6, 0.1)
        # Blipping cursor feel - keeping it solid for icon
        if 0.4 <= nx <= 0.6 and 0.08 <= ny <= 0.12:
            final_color = col_white

        return final_color

    # Fill buffer (bottom-up for BMP)
    idx = 0
    for y in range(size):
        for x in range(size):
            r, g, b, a = get_color(x, y)
            # BGRA
            buffer[idx] = b
            buffer[idx+1] = g
            buffer[idx+2] = r
            buffer[idx+3] = a
            idx += 4
            
    return buffer

def create_bmp_bytes(size, pixels):
    # DIB Header (40 bytes for BITMAPINFOHEADER)
    header_size = 40
    width = size
    height = size * 2 # Height * 2 usually for masks in ICO, but standard PNG/BMP inside ICO follows specific rules. 
    # Actually for 32bpp BMP in ICO, the height in standard BMP header is often 'size * 2' providing the AND mask, 
    # OR it's just 'size' and we omit the separate mask if we use alpha channel.
    # Modern Windows (XP+) supports 32bpp BGRA with embedded alpha.
    # We will use the simple BITMAPINFOHEADER with 32bpp and no compression (BI_RGB).
    
    # Correction: For ICO, the bitmap header height is indeed usually height * 2 to account for the AND mask (1bpp) 
    # that immediately follows the XOR mask (pixel data). Even if we use 32bpp alpha.
    # So we need to generate the AND mask logic too.
    
    height_hdr = size * 2
    planes = 1
    bpp = 32
    compression = 0
    image_size = len(pixels) + (size * size // 8) # XOR data + AND mask (1 bit per pixel)
    # AND Mask row stride must be multiple of 4 bytes.
    
    # Calculate AND mask size
    # Row width in bits = size. Row width in bytes = ceil(size/8). 
    # Stride = align 4 bytes.
    row_bytes_and = ((size + 31) // 32) * 4
    mask_size = row_bytes_and * size
    
    image_size = len(pixels) + mask_size
    
    x_pels_per_meter = 0
    y_pels_per_meter = 0
    clr_used = 0
    clr_important = 0
    
    header = struct.pack('<IIIHHIIIIII',
        header_size, width, height_hdr, planes, bpp, compression,
        image_size, x_pels_per_meter, y_pels_per_meter, clr_used, clr_important
    )
    
    # AND Mask (all 0 for fully transparent check? No, 0 means opaque in AND mask usually. 
    # But for 32bpp with alpha, Windows ignores AND mask if Alpha is used, BUT logic still requires it present.)
    # We'll set it to all 0s (opaque) just to be safe, relying on Alpha channel.
    and_mask = bytearray(mask_size)
    
    return header + pixels + and_mask

if __name__ == "__main__":
    target_path = r"e:\Docker\AgentArena\arena.ico"
    # Create directory if needed
    os.makedirs(os.path.dirname(target_path), exist_ok=True)
    create_ico(target_path)
