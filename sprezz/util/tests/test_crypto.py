import unittest

from ..crypto import PersistentRSAKey


class Test_crypto(unittest.TestCase):
    def test_sign_verify(self):
        prv_key = PersistentRSAKey()
        prv_key.generate_keypair()
        message = 'hi there'
        sig = prv_key.sign_message(message)
        self.assertTrue(prv_key.verify_message(message, sig))

        pub_pem = prv_key.export_public_key()
        pub_key = PersistentRSAKey(extern_public_key=pub_pem)
        self.assertTrue(pub_key.verify_message(message, sig))

        pub2_key = prv_key.get_public_key()
        self.assertTrue(pub2_key.verify_message(message, sig))
