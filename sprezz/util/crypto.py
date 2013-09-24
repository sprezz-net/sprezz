import logging

from Crypto import Random
from Crypto.Cipher import AES
from Crypto.Cipher import PKCS1_v1_5 as Cip_PKCS1_v1_5
from Crypto.Hash import SHA256
from Crypto.Padding import pad, unpad
from Crypto.PublicKey import RSA
from Crypto.Signature import PKCS1_v1_5 as Sig_PKCS1_v1_5
from persistent import Persistent

from .base64 import base64_url_encode, base64_url_decode


log = logging.getLogger(__name__)


# AES256 uses a key size of 32 bytes
AES_KEY_SIZE = 32


def aes256_cbc_encrypt(message, randfunc=None):
    """Encrypt message using AES-256 in Cipher-block chaining (CBC) mode.

    Prior to encryption, the message is padded using :func:`pkcs7_pad`.

    Returns a tuple of ``cipher_text``, ``key``, and ``iv``.
    """
    if randfunc is None:
        randfunc = Random.new().read
    key = randfunc(AES_KEY_SIZE)
    iv = randfunc(AES.block_size)
    cipher = AES.new(key, AES.MODE_CBC, iv)
    cipher_text = cipher.encrypt(pad(message, AES.block_size, 'pkcs7'))
    return (cipher_text, key, iv)


def aes256_cbc_decrypt(cipher_text, key, iv):
    """Decrypt ``cipher_text`` using AES-256 in Cipher-block chaining (CBC)
    mode with parameters ``key`` and ``iv``. The resulting message is unpadded
    using :func:`pkcs7_unpad`.
    """
    cipher = AES.new(key, AES.MODE_CBC, iv)
    return unpad(cipher.decrypt(cipher_text), AES.block_size, 'pkcs7')


class PersistentRSAKey(Persistent):
    """Class ``PersistentRSAKey`` stores public/private RSA keys as
    :term:`PEM` encoded strings.
    """
    def __init__(self, extern_public_key=None, extern_private_key=None):
        super().__init__()
        self._private_key = extern_private_key
        self._public_key = extern_public_key

    def generate_keypair(self, bits=4096, randfunc=None):
        if randfunc is None:
            randfunc = Random.new().read
        key = RSA.generate(bits, randfunc)
        self._private_key = key.exportKey(format='PEM',
                                          passphrase=None, pkcs=1)
        self._public_key = key.publickey().exportKey(format='PEM',
                                                     passphrase=None)
        self._v_key = key

    def _import_key(self):
        """Construct the cached RSA key object."""
        key = None
        if self._private_key is not None:
            key = RSA.importKey(self._private_key, passphrase=None)
        elif self._public_key is not None:
            key = RSA.importKey(self._public_key, passphrase=None)
        self._v_key = key
        return key

    def get_public_key(self):
        """Return a clone of ``PersistentRSAKey`` using the public key."""
        return PersistentRSAKey(extern_public_key=self._public_key)

    def export_public_key(self):
        if self._public_key is None:
            key = self._import_key()
            self._public_key = key.publickey().exportKey(format='PEM',
                                                         passphrase=None)
        pub_key = self._public_key[:].decode("latin-1")
        pub_key = pub_key.replace('-----BEGIN RSA PUBLIC KEY-----',
                                  '-----BEGIN PUBLIC KEY-----')
        pub_key = pub_key.replace('-----END RSA PUBLIC KEY-----',
                                  '-----END PUBLIC KEY-----')
        return pub_key

    def sign(self, message):
        try:
            key = self._v_key
        except AttributeError:
            key = self._import_key()
        if key is None:
            raise TypeError('No key')
        h = SHA256.new(message)
        signer = Sig_PKCS1_v1_5.new(key)
        return signer.sign(h)

    def verify(self, message, signature):
        try:
            key = self._v_key
        except AttributeError:
            key = self._import_key()
        if key is None:
            raise TypeError('No key')
        h = SHA256.new(message)
        verifier = Sig_PKCS1_v1_5.new(key)
        return verifier.verify(h, signature)

    def encrypt(self, message):
        try:
            key = self._v_key
        except AttributeError:
            key = self._import_key()
        if key is None:
            raise TypeError('No key')
        cipher = Cip_PKCS1_v1_5.new(key)
        return cipher.encrypt(message)

    def decrypt(self, cipher_text):
        try:
            key = self._v_key
        except AttributeError:
            key = self._import_key()
        if key is None:
            raise TypeError('No key')
        cipher = Cip_PKCS1_v1_5.new(key)
        message = cipher.decrypt(cipher_text, None)
        if message is None:
            raise ValueError
        else:
            return message

    def aes_encapsulate(self, data):
        result = {}
        cipher_text, key, iv = aes256_cbc_encrypt(data)
        result['data'] = base64_url_encode(cipher_text)
        result['key'] = base64_url_encode(self.encrypt(key))
        result['iv'] = base64_url_encode(self.encrypt(iv))
        return result

    def aes_decapsulate(self, data):
        key = self.decrypt(base64_url_decode(data['key']))
        iv = self.decrypt(base64_url_decode(data['iv']))
        return aes256_cbc_decrypt(base64_url_decode(data['data']), key, iv)
