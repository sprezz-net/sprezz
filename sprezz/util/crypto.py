import logging

from Crypto import Random
from Crypto.Cipher import AES
from Crypto.Cipher import PKCS1_v1_5 as Cip_PKCS1
from Crypto.Hash import SHA, SHA256
from Crypto.PublicKey import RSA
from Crypto.Signature import PKCS1_v1_5 as Sig_PKCS1
from persistent import Persistent

from .base64 import base64_url_encode, base64_url_decode


log = logging.getLogger(__name__)


AES_KEY_SIZE = 32


pkcs7_pad = lambda m, bs: m + (bs - len(m) % bs) * bytes([bs - len(m) % bs])


pkcs7_unpad = lambda m: m[0:-m[-1]]


def aes256_cbc_encrypt(message):
    key = Random.new().read(AES_KEY_SIZE)
    iv = Random.new().read(AES.block_size)
    cipher = AES.new(key, AES.MODE_CBC, iv)
    cipher_text = cipher.encrypt(pkcs7_pad(message, AES.block_size))
    return (cipher_text, key, iv)


def aes256_cbc_decrypt(cipher_text, key, iv):
    cipher = AES.new(key, AES.MODE_CBC, iv)
    return pkcs7_unpad(cipher.decrypt(cipher_text))


class PersistentRSAKey(Persistent):
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

    def _import_keys(self):
        if self._private_key is not None:
            self._v_key = RSA.importKey(self._private_key,
                                        passphrase=None)
        elif self._public_key is not None:
            self._v_key = RSA.importKey(self._public_key,
                                        passphrase=None)
        else:
            self._v_key = None

    def get_public_key(self):
        return PersistentRSAKey(extern_public_key=self._public_key)

    def export_public_key(self):
        pub_key = self._public_key[:].decode("latin-1")
        pub_key = pub_key.replace('-----BEGIN RSA PUBLIC KEY-----',
                                  '-----BEGIN PUBLIC KEY-----')
        pub_key = pub_key.replace('-----END RSA PUBLIC KEY-----',
                                  '-----END PUBLIC KEY-----')
        return pub_key

    def sign_message(self, message):
        try:
            if self._v_key is None:
                self._import_keys()
        except AttributeError:
            self._import_keys()
        if (self._v_key is not None) and (
                self._v_key.can_sign() and self._v_key.has_private()):
            h = SHA256.new(message)
            signer = Sig_PKCS1.new(self._v_key)
            return signer.sign(h)
        return None  # raise TypeError

    def verify_message(self, message, signature):
        try:
            if self._v_key is None:
                self._import_keys()
        except AttributeError:
            self._import_keys()
        if self._v_key is not None:
            h = SHA256.new(message)
            verifier = Sig_PKCS1.new(self._v_key)
            return verifier.verify(h, signature)
        return False  # raise TypeError

    def encrypt(self, message):
        try:
            if self._v_key is None:
                self._import_keys()
        except AttributeError:
            self._import_keys()
        if (self._v_key is not None) and self._v_key.can_encrypt():
            cipher = Cip_PKCS1.new(self._v_key)
            return cipher.encrypt(message)
        raise TypeError

    def decrypt(self, cipher_text):
        try:
            if self._v_key is None:
                self._import_keys()
        except AttributeError:
            self._import_keys()
        if (self._v_key is not None) and self._v_key.has_private():
            cipher = Cip_PKCS1.new(self._v_key)
            message = cipher.decrypt(cipher_text, None)
            if message is None:
                raise ValueError
            else:
                return message
        raise TypeError

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
