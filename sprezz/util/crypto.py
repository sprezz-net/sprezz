from Crypto import Random
from Crypto.Hash import SHA256
from Crypto.PublicKey import RSA
from Crypto.Signature import PKCS1_v1_5
from persistent import Persistent


class PersistentRSAKey(Persistent):
    def __init__(self, extern_public_key=None):
        super().__init__()
        self._private_key = None
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
            signer = PKCS1_v1_5.new(self._v_key)
            return signer.sign(h)
        return None

    def verify_message(self, message, signature):
        try:
            if self._v_key is None:
                self._import_keys()
        except AttributeError:
            self._import_keys()
        if self._v_key is not None:
            h = SHA256.new(message)
            verifier = PKCS1_v1_5.new(self._v_key)
            if verifier.verify(h, signature):
                return True
        return False
