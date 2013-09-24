import logging
import unittest

from unittest.mock import patch, Mock


log = logging.getLogger()
log.level = logging.DEBUG


class TestCrypto(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # import sys
        # stream_handler = logging.StreamHandler(sys.stdout)
        # log.addHandler(stream_handler)
        pass

    def setUp(self):
        self.pub_pem = b"""-----BEGIN RSA PUBLIC KEY-----
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA4Fu2JFk2Mbiy435jZjmb
S9XWiprJmUW4/t/WNDbruwJoGM3jcjE+bTxkyNyO4qRwPvhPeoU7klgD1n6z0m6v
JQPsJDfR0LFr2zKDmUh2ybr9jEgmKEIku5Rma6FfwjDCxRzy810XWaa8cLhF5Iko
8gzNiKbc8p2YOangLvpRAyAdm6kl+4SXvoYVf8c9UP8GD4eTBNfySOg7W5qokx82
X0lUuxJYlPa/24ZLi0no/BX699HloqgSxnRTzCbpT7Bq1R4b2uoslevABzTBowGx
6uk4j7z/cTvxyiMO2ikJxPZY7AzAMaoil4FUyqjNUoypvfEpuSHL26EvaM+YEDpx
G7QrNgsZVl9HmBcQorHdZw+IBx5QhBg5jViX2YzTYTv90Sps0PmgAuUY4T3GmzXi
BmZyEGxDHKVGJjiD9eIA1tH1p3OGKpN2lF3P7gKXdun+QyaHyjnrt6/PFC1D3d5w
tDt18gN/uCCiE0XHpG3rr9X9pcTn1cIIC0NpH2lIhWv8Gb+gbOLhpHIMxmRxih+w
omCvwZc+vzcgVYfLaKBOcqdDgaX+wfA6Vsw/FPIcDDZHsQ68uySXHcaW8qINCIiO
BE6UEqaz5n1hWL92P79MEMwAm47fWIiYQE5KsjZqEN8IGSDOzw8uekIDCQ94DvZH
zlbZ0Xv9yEx1EGg8ZcD7XG8CAwEAAQ==
-----END RSA PUBLIC KEY-----"""
        self.prv_pem = b"""-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----"""

    def _makeOne(self):
        from sprezz.util.crypto import PersistentRSAKey
        key = PersistentRSAKey(extern_public_key=self.pub_pem,
                               extern_private_key=self.prv_pem)
        return key

    @patch('sprezz.util.crypto.RSA.generate')
    def test_generator(self, rsa_mock):
        from sprezz.util.crypto import PersistentRSAKey
        key = self._makeOne()
        internal_key = key._import_key()
        rsa_mock.return_value = internal_key
        gen_key = PersistentRSAKey()
        gen_key.generate_keypair(randfunc=Mock)
        # Check if key is generated with 4096 bits
        rsa_mock.assert_called_once_with(4096, Mock)
        self.assertEquals(gen_key._private_key, self.prv_pem)
        self.assertEquals(gen_key._public_key, self.pub_pem)

    def test_sign_verify(self):
        key = self._makeOne()
        message = 'hi there'
        sig = key.sign(message)
        self.assertTrue(key.verify(message, sig))
        self.assertFalse(key.verify('hi there!', sig))

    def test_sign_no_prv(self):
        key = self._makeOne()
        pub_key = key.get_public_key()
        message = 'hi there'
        with self.assertRaises(TypeError):
            pub_key.sign(message)

    def test_sign_verify_pub(self):
        key = self._makeOne()
        message = 'hi there'
        sig = key.sign(message)
        pub_key = key.get_public_key()
        self.assertTrue(pub_key.verify(message, sig))
        self.assertFalse(pub_key.verify('hi there!', sig))

    def test_export_public_key(self):
        key = self._makeOne()
        pub_pem = key.export_public_key()
        self.assertIs(type(pub_pem), str)
        self.assertTrue(pub_pem.startswith('-----BEGIN PUBLIC KEY-----'))
        self.assertTrue(pub_pem.endswith('-----END PUBLIC KEY-----'))
        pub_pem = pub_pem.replace('-----BEGIN PUBLIC KEY-----',
                                  '-----BEGIN RSA PUBLIC KEY-----')
        pub_pem = pub_pem.replace('-----END PUBLIC KEY-----',
                                  '-----END RSA PUBLIC KEY-----')
        self.assertEqual(pub_pem, self.pub_pem.decode('latin1'))

    def test_encrypt_decrypt(self):
        key = self._makeOne()
        pub_key = key.get_public_key()
        message = 'hi there'.encode('utf-8')
        ct = pub_key.encrypt(message)
        result = key.decrypt(ct)
        self.assertEqual(result, message)
        with self.assertRaises(TypeError):
            pub_key.decrypt(ct)
        with self.assertRaises(ValueError):
            key.decrypt(ct + b'MISALIGN')

    def test_prv_encrypt_decrypt(self):
        key = self._makeOne()
        message = 'hi there'.encode('utf-8')
        ct = key.encrypt(message)
        result = key.decrypt(ct)
        self.assertEqual(result, message)

    def test_aes(self):
        key = self._makeOne()
        pub_key = key.get_public_key()
        message = 'hi there'.encode('utf-8')
        data = pub_key.aes_encapsulate(message)
        self.assertIn('data', data)
        self.assertIn('key', data)
        self.assertIn('iv', data)
        result = key.aes_decapsulate(data)
        self.assertEqual(result, message)
        with self.assertRaises(TypeError):
            pub_key.aes_decapsulate(data)

    def test_prv_aes(self):
        key = self._makeOne()
        message = 'hi there'.encode('utf-8')
        data = key.aes_encapsulate(message)
        self.assertIn('data', data)
        self.assertIn('key', data)
        self.assertIn('iv', data)
        result = key.aes_decapsulate(data)
        self.assertEqual(result, message)

    def test_pkcs7_pad_unpad(self):
        from sprezz.util.crypto import pkcs7_pad, pkcs7_unpad
        data = {b'message': b'message\x09\x09\x09\x09\x09\x09\x09\x09\x09',
                b'0123456789ABCDE': b'0123456789ABCDE\x01',
                b'0123456789ABCDEF': b'0123456789ABCDEF'
                                     b'\x10\x10\x10\x10\x10\x10\x10\x10'
                                     b'\x10\x10\x10\x10\x10\x10\x10\x10'}
        for k,v in data.items():
            result = pkcs7_pad(k, 16)
            self.assertEqual(result, v)
            # last byte is padding byte and padding length
            n = result[-1]
            # check that the last byte before padding length is different
            self.assertNotEqual(result[-n-1], n)
            self.assertEqual(result.rstrip(bytes([n])), k)
            self.assertEqual(pkcs7_unpad(v), k)
