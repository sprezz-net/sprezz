import logging
import pprint
import sys
import unittest

from pyramid import testing
from unittest.mock import patch, call, Mock


log = logging.getLogger()
log.level = logging.DEBUG


class TestZot(unittest.TestCase):
    def setUp(self):
        #stream_handler = logging.StreamHandler(sys.stdout)
        #log.addHandler(stream_handler)
        self.config = testing.setUp()

    def tearDown(self):
        testing.tearDown()

    def _makeOne(self):
        from ..zot import Zot
        return Zot()

    @patch('sprezz.zot.zot.PersistentRSAKey', spec=True)
    def test_after_create(self, rsa_mock):
        """Test that all content containers are created."""
        inst = self._makeOne()
        Z = {}
        def add(name, val, registry=None):
            Z[name] = val
        inst.add = add
        ob = testing.DummyResource()
        content = DummySingleContentRegistry(ob)
        registry = testing.DummyResource()
        registry.content = content
        inst.after_create(None, registry)

        # Test generation of site keys
        calls = [call(), call().generate_keypair(), call().get_public_key()]
        rsa_mock.assert_has_calls(calls)

        # Test creation of containers
        self.assertEqual(Z['site'], ob)
        self.assertEqual(Z['hub'], ob)
        self.assertEqual(Z['channel'], ob)
        self.assertEqual(Z['xchannel'], ob)
        self.assertEqual(Z['post'], ob)
        self.assertEqual(Z['poco'], ob)

    @patch('sprezz.zot.zot.PersistentRSAKey', spec=True)
    def test_add_channel(self, rsa_mock):
        inst = self._makeOne()
        inst['channel'] = DummyFolder()
        inst['xchannel'] = DummyFolder()
        inst['hub'] = DummyFolder()
        inst._create_channel_guid = Mock(name='create_channel_guid')
        inst._create_channel_signature = Mock(name='create_channel_signature')
        inst._create_channel_hash = Mock(name='create_channel_hash',
                                         return_value='hash')
        inst._public_site_key = Mock(name='public_site_key',
                                     return_value='key')

        ob_chan = Mock(name='ZotLocalChannel')
        ob_xchan = Mock(name='ZotLocalXChannel')
        ob_hub = Mock(name='ZotLocalHub')
        content = DummyDictContentRegistry({'ZotLocalChannel': ob_chan,
                                            'ZotLocalXChannel': ob_xchan,
                                            'ZotLocalHub': ob_hub})
        registry = testing.DummyResource()
        registry.content = content
        result = inst.add_channel('admin', 'Adminstrator',
                                  registry=registry)

        # Test channel key generation calls
        calls = [call(), call().generate_keypair(), call().get_public_key()]
        rsa_mock.assert_has_calls(calls)

        # Check helpers
        self.assertEqual(inst._create_channel_guid.call_count, 1)
        self.assertEqual(inst._create_channel_signature.call_count, 1)
        self.assertEqual(inst._create_channel_hash.call_count, 1)

        # Check that the objects are added to the containers
        self.assertEqual(inst['channel']['admin'], ob_chan)
        self.assertEqual(inst['xchannel']['hash'], ob_xchan)
        self.assertEqual(inst['hub']['hash'], ob_hub)
        self.assertEqual(result, ob_chan)


class DummySingleContentRegistry(object):
    def __init__(self, result):
        self.result = result

    def create(self, content_type, *arg, **kw):
        return self.result


class DummyDictContentRegistry(object):
    def __init__(self, result):
        self.result = result

    def create(self, content_type, *arg, **kw):
        return self.result[content_type]


class DummyFolder(testing.DummyResource):
    def __init__(self):
        super().__init__()
        self.data = {}

    def add(self, name, val, registry=None):
        self.data[name] = val
        return name

    def __getitem__(self, name):
        return self.data[name]
