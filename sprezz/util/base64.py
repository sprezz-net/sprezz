from base64 import urlsafe_b64encode, urlsafe_b64decode


def base64_url_encode(data, strip_padding=True):
    """Encode binary data as url safe base64

    Argument data should be binary, return value is a string.
    Boolean argument strip_padding strips '=' padding characters from the
    right-hand side of the output. Default is True.
    """
    out = urlsafe_b64encode(data).decode('ascii')
    if strip_padding:
        out = out.rstrip('=')
    return out


def base64_url_decode(string):
    """Decode a base64 url encoded string into binary data

    Argument string is padded with '=' when not divisable by four.
    """
    length = len(string)
    padding = length % 4
    if padding > 0:
        string = string.ljust(length + padding, '=')
    return urlsafe_b64decode(string)
