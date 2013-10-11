import unittest

from sprezz.util import create_channel_hash


class TestUtilZot(unittest.TestCase):
    def test_create_channel_hash(self):
        hashed = create_channel_hash('guid', 'signature')
        # Base64 url encoded Whirlpool hashed 'guidsignature'
        self.assertEqual(hashed, 'EtDtUr7cPtjoXQjDq_d68-AIFRNetS-KudlogIffsYy'
                                 'kH66_LwsOKee3dtoV7KDg1V08s_6K4TVIgo2O5KvNiQ')
