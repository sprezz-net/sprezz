import whirlpool

from .base64 import base64_url_encode


def create_channel_hash(guid, signature):
    """Create base64 encoded channel hash.

    Arguments ``guid`` and ``signature`` must be base64 encoded.
    """
    message = '{0}{1}'.format(guid, signature).encode('ascii')
    wp = whirlpool.new(message)
    return base64_url_encode(wp.digest())
