import unittest

from ..util import (
        base64_url_encode,
        base64_url_decode,
        )


data = {
        'empty'  : b'',
        'f'      : b'f',
        'fo'     : b'fo',
        'foo'    : b'foo',
        'foob'   : b'foob',
        'fooba'  : b'fooba',
        'foobar' : b'foobar',
        'binary' : b'fo\xaf\xfea',
        }

result = {
        'empty'  : '',
        'f'      : 'Zg==',
        'fo'     : 'Zm8=',
        'foo'    : 'Zm9v',
        'foob'   : 'Zm9vYg==',
        'fooba'  : 'Zm9vYmE=',
        'foobar' : 'Zm9vYmFy',
        'binary' : 'Zm-v_mE=',
        }

result_np = {
        'empty'  : '',
        'f'      : 'Zg',
        'fo'     : 'Zm8',
        'foo'    : 'Zm9v',
        'foob'   : 'Zm9vYg',
        'fooba'  : 'Zm9vYmE',
        'foobar' : 'Zm9vYmFy',
        'binary' : 'Zm-v_mE',
        }


class Test_base64_url_encode(unittest.TestCase):
    def test_padding(self):
        for k in data.keys():
            out = base64_url_encode(data[k], False)
            self.assertEqual(out, result[k])

    def test_nopadding(self):
        for k in data.keys():
            out = base64_url_encode(data[k], True)
            self.assertEqual(out, result_np[k])


class Test_base64_url_decode(unittest.TestCase):
    def test_padding(self):
        for k in result.keys():
            out = base64_url_decode(result[k])
            self.assertEqual(out, data[k])

    def test_nopadding(self):
        for k in result_np.keys():
            out = base64_url_decode(result_np[k])
            self.assertEqual(out, data[k])
