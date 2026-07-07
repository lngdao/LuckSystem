import core
from base.cartagrahd import *

def Init():
    # core.Charset_UTF8
    # core.Charset_Unicode
    # core.Charset_SJIS
    core.set_config(expr_charset=core.Charset_UTF8,
                    text_charset=core.Charset_Unicode,
                    default_export=True)

def MESSAGE():
    core.read_uint16(True)
    core.read_len_str(core.text)  # Japanese
    core.read_len_str(core.expr)  # English
    core.read(False)
    core.end()

def SELECT():
    core.read_uint16()
    core.read_uint16()
    core.read_uint16()
    core.read_uint16()
    core.read_len_str(core.text)
    core.read(True)
    core.end()

def DIALOG():
    core.read(False)
    core.end()

def LOG_BEGIN():
    core.read_uint8(False)
    core.read_uint8(False)
    core.read_uint8(False)
    core.read_len_str(core.text)  # Japanese
    core.read_len_str(core.text)  # English
    core.read_len_str(core.text)  # Simplified Chinese
    core.read(False)
    core.end()
