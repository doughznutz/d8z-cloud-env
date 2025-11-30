import logging
import sys

def setup_logging(debug=False, log_file=None, log_level="INFO"):
    """Configures logging for the application."""
    level = logging.DEBUG if debug else getattr(logging, log_level.upper(), logging.INFO)
    
    formatter = logging.Formatter('%(asctime)s - %(name)s - %(levelname)s - %(message)s')
    
    # Get the root logger
    root_logger = logging.getLogger()
    root_logger.setLevel(level)
    
    # Clear existing handlers to avoid duplicate logs
    if root_logger.hasHandlers():
        root_logger.handlers.clear()
        
    # Console handler to stderr
    stream_handler = logging.StreamHandler(sys.stderr)
    stream_handler.setFormatter(formatter)
    root_logger.addHandler(stream_handler)
    
    # File handler
    if log_file:
        file_handler = logging.FileHandler(log_file)
        file_handler.setFormatter(formatter)
        root_logger.addHandler(file_handler)
        
    logging.getLogger(__name__).info(f"Logging configured at level {logging.getLevelName(level)}")
