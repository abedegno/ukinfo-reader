"""Read supported UK-Info Disk databases with native Python."""

from .codecs import FormatError
from .editions import SUPPORTED_EDITIONS, UnsupportedEditionError
from .reader import Database

__all__ = ["Database", "FormatError", "SUPPORTED_EDITIONS", "UnsupportedEditionError"]
__version__ = "0.4.0"
