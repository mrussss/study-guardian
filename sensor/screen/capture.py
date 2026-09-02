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


def hamming_distance(hash1: str, hash2: str) -> int:
    """Calculate the Hamming distance between two hex hashes."""
    try:
        x = int(hash1, 16) ^ int(hash2, 16)
        return bin(x).count('1')
    except Exception:
        return 64


class ScreenCapturer:
    def __init__(self, change_threshold: int = 4):
        self.last_hash: Optional[str] = None
        self.last_capture_time: Optional[float] = None
        self.change_threshold = change_threshold
        self.sct = None
        if HAS_CAPTURE_DEPS:
            try:
                self.sct = mss.mss()
            except Exception:
                self.sct = None

    def compute_dhash(self, image: 'Image.Image', hash_size: int = 8) -> str:
        """Compute difference hash (dHash) of an image."""
        resized = image.convert('L').resize((hash_size + 1, hash_size), Image.Resampling.BILINEAR)
        pixels = list(resized.getdata())
        
        diff = []
        width = hash_size + 1
        for row in range(hash_size):
            row_start = row * width
            for col in range(hash_size):
                left = pixels[row_start + col]
                right = pixels[row_start + col + 1]
                diff.append(left > right)
        
        decimal_val = 0
        for idx, bit in enumerate(diff):
            if bit:
                decimal_val |= 1 << idx
        return f"{decimal_val:016x}"

    def capture(self, monitor_idx: int = 1, include_analysis_image: bool = False, max_width: int = 960) -> Dict[str, Any]:
        now_iso = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        
        if not HAS_CAPTURE_DEPS or self.sct is None:
            fake_hash = hashlib.sha256(f"stub-{time.time()}".encode()).hexdigest()[:16]
            changed = True
            if self.last_hash is not None:
                dist = hamming_distance(self.last_hash, fake_hash)
                changed = dist >= self.change_threshold
            self.last_hash = fake_hash
            return {
                "timestamp": now_iso,
                "monitor": monitor_idx,
                "changed": changed,
                "hash": fake_hash,
                "distance": 0 if not changed else 8,
                "is_stub": True,
                "error": None,
            }

        try:
            monitors = self.sct.monitors
            if monitor_idx >= len(monitors):
                target_mon = monitors[1] if len(monitors) > 1 else monitors[0]
            elif monitor_idx < 0:
                target_mon = monitors[0] # All monitors virtual desktop
            else:
                target_mon = monitors[monitor_idx]

            sct_img = self.sct.grab(target_mon)
            img = Image.frombytes("RGB", sct_img.size, sct_img.bgra, "raw", "BGRX")

            current_hash = self.compute_dhash(img)
            
            if self.last_hash is None:
                changed = True
                dist = 64
            else:
                dist = hamming_distance(self.last_hash, current_hash)
                changed = (dist >= self.change_threshold)

            self.last_hash = current_hash
            self.last_capture_time = time.time()

            result = {
                "timestamp": now_iso,
                "monitor": monitor_idx,
                "changed": changed,
                "hash": current_hash,
                "distance": dist,
                "is_stub": False,
                "error": None,
            }

            if include_analysis_image:
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
                "distance": 0,
                "is_stub": False,
                "error": str(e),
            }
