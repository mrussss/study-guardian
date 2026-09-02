import hashlib
import io
import time
from typing import Optional, Tuple, Dict, Any

try:
    import mss
    import mss.tools
    from PIL import Image
    HAS_CAPTURE_DEPS = True
except ImportError:
    HAS_CAPTURE_DEPS = False


class ScreenCapturer:
    def __init__(self):
        self.last_hash: Optional[str] = None
        self.last_capture_time: Optional[float] = None
        self.sct = None
        if HAS_CAPTURE_DEPS:
            try:
                self.sct = mss.mss()
            except Exception:
                self.sct = None

    def compute_dhash(self, image: 'Image.Image', hash_size: int = 8) -> str:
        """Compute difference hash (dHash) of an image."""
        # Convert to grayscale and resize to (hash_size + 1, hash_size)
        resized = image.convert('L').resize((hash_size + 1, hash_size), Image.Resampling.BILINEAR)
        pixels = list(resized.getdata())
        
        # Compare adjacent pixels in each row
        diff = []
        width = hash_size + 1
        for row in range(hash_size):
            row_start = row * width
            for col in range(hash_size):
                left = pixels[row_start + col]
                right = pixels[row_start + col + 1]
                diff.append(left > right)
        
        # Convert boolean list to hex string
        decimal_val = 0
        for idx, bit in enumerate(diff):
            if bit:
                decimal_val |= 1 << idx
        return f"{decimal_val:016x}"

    def capture(self, monitor_idx: int = 1, include_analysis_image: bool = False, max_width: int = 960) -> Dict[str, Any]:
        now_iso = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        
        if not HAS_CAPTURE_DEPS or self.sct is None:
            # Stub mode when mss / PIL not available (e.g. headless CI / unit testing without GUI)
            fake_hash = hashlib.sha256(f"stub-{time.time()}".encode()).hexdigest()[:16]
            changed = self.last_hash is None or self.last_hash != fake_hash
            self.last_hash = fake_hash
            return {
                "timestamp": now_iso,
                "monitor": monitor_idx,
                "changed": changed,
                "hash": fake_hash,
                "is_stub": True,
                "error": None,
            }

        try:
            monitors = self.sct.monitors
            if monitor_idx >= len(monitors):
                target_mon = monitors[1] if len(monitors) > 1 else monitors[0]
            else:
                target_mon = monitors[monitor_idx]

            sct_img = self.sct.grab(target_mon)
            img = Image.frombytes("RGB", sct_img.size, sct_img.bgra, "raw", "BGRX")

            current_hash = self.compute_dhash(img)
            changed = (self.last_hash is None) or (self.last_hash != current_hash)
            self.last_hash = current_hash
            self.last_capture_time = time.time()

            result = {
                "timestamp": now_iso,
                "monitor": monitor_idx,
                "changed": changed,
                "hash": current_hash,
                "is_stub": False,
                "error": None,
            }

            if include_analysis_image:
                # Resize if wider than max_width
                if img.width > max_width:
                    scale = max_width / float(img.width)
                    new_size = (max_width, int(img.height * scale))
                    img = img.resize(new_size, Image.Resampling.BILINEAR)
                
                buf = io.BytesIO()
                img.save(buf, format="JPEG", quality=75)
                import base64
                result["analysis_image"] = base64.b64encode(buf.getvalue()).decode("ascii")

            return result

        except Exception as e:
            return {
                "timestamp": now_iso,
                "monitor": monitor_idx,
                "changed": False,
                "hash": "",
                "is_stub": False,
                "error": str(e),
            }
