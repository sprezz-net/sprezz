from logging import getLogger
from secrets import SystemRandom


log = getLogger(__name__)


RNG = SystemRandom()


UNICODE_ASCII_CHARACTER_SET = ('abcdefghijklmnopqrstuvwxyz'
                               'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
                               '0123456789')


CLIENT_ID_CHARACTER_SET = (r' !"#$%&\'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMN'
                           'OPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}')


def generate_token(length=30, chars=UNICODE_ASCII_CHARACTER_SET):
    """Generates a non-guessable OAuth token
    OAuth (1 and 2) does not specify the format of tokens except that they
    should be strings of random characters. Tokens should not be guessable
    and entropy when generating the random characters is important. Which is
    why SystemRandom is used instead of the default random.choice method.
    """
    return ''.join(RNG.choice(chars) for x in range(length))


def generate_client_id(length=30, chars=CLIENT_ID_CHARACTER_SET):
    """Generates an OAuth client_id
    OAuth 2 specify the format of client_id in
    https://tools.ietf.org/html/rfc6749#appendix-A.
    """
    return generate_token(length, chars)


def generate_client_secret(length=128, chars=CLIENT_ID_CHARACTER_SET):
    """Generates an OAuth client_secret
    OAuth 2 specify the format of client_secret in
    https://tools.ietf.org/html/rfc6749#appendix-A.
    """
    return generate_token(length, chars)
