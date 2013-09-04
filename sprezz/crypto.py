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

    def generate_keypair(self, bits=4096):
        g = Random.new().read
        self._v_key = RSA.generate(bits, g)
        self._private_key = self._v_key.exportKey(format='PEM',
                passphrase=None, pkcs=1)
        self._public_key = self._v_key.publickey().exportKey(format='PEM',
                passphrase=None, pkcs=1)

    def _import_keys(self):
        if self._private_key is not None:
            self._v_key = RSA.importKey(self._private_key,
                    passphrase=None)
        elif self._public_key is not None:
            self._v_key = RSA.importKey(self._public_key,
                    passphrase=None)
        else:
            self._v_key = None

    def export_public_key(self):
        return self._public_key

    def sign_message(self, message):
        if not hasattr(self, '_v_key') or self._v_key is None:
            self._import_keys()
        if (self._v_key is not None) and (
                self._v_key.can_sign() and self._v_key.has_private()):
            h = SHA256.new(message)
            signer = PKCS1_v1_5.new(self._v_key)
            return signer.sign(h)
        return None

    def verify_message(self, message, signature):
        if not hasattr(self, '_v_key') or self._v_key is None:
            self._import_keys()
        if self._v_key is not None:
            h = SHA256.new(message)
            verifier = PKCS1_v1_5.new(self._v_key)
            if verifier.verify(h, signature):
                return True
        return False
