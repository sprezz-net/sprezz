import logging
import unittest
import sys

from pyramid import testing
from unittest.mock import patch

from ..testing import create_single_content_registry, DummyFolder


log = logging.getLogger()
log.level = logging.DEBUG


class TestWellKnown(unittest.TestCase):
    def setUp(self):
        self.config = testing.setUp()

    def tearDown(self):
        testing.tearDown()

    def _makeOne(self):
        from sprezz.wellknown import WellKnown
        return WellKnown()

    def test_after_create(self):
        inst = self._makeOne()
        Z = {}
        def add(name, val, registry=None):
            Z[name] = val
        inst.add = add
        ob = testing.DummyResource()
        registry = create_single_content_registry(ob)
        inst.after_create(None, registry)
        self.assertEqual(Z['zot-info'], ob)


class TestZotInfoProtocol(unittest.TestCase):
    def setUp(self):
        request = testing.DummyRequest()
        self.config = testing.setUp(request=request)

    def tearDown(self):
        testing.tearDown()

    def _makeOne(self, context, request):
        from sprezz.wellknown import ZotInfoProtocol
        return ZotInfoProtocol(context, request)

    def _makeResourceTree(self, context):
        root = DummyFolder()
        root['context'] = context
        zot = root['zot'] = DummyFolder()
        zot.__is_service__ = True
        zot['channel'] = DummyFolder()
        zot['xchannel'] = DummyFolder()
        zot['hub'] = DummyFolder()
        return zot

    def test_zot_info_no_key(self):
        context = testing.DummyResource()
        request = testing.DummyRequest(params={'target': 'target'})
        request.graph = None
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertFalse(result['success'])
        self.assertEquals(result['message'],
                          'No key or target signature supplied.')

    @patch('sprezz.wellknown.PersistentRSAKey.verify_message',
           return_value=False)
    def test_zot_info_invalid_sig(self, rsa_mock):
        context = testing.DummyResource()
        request = testing.DummyRequest(params={'target': 'target',
                                               'target_sig': 'sig',
                                               'key': 'key'},
                                       method='POST')
        request.graph = None
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertFalse(result['success'])
        self.assertEquals(result['message'],
                          'Invalid target signature.')

    @patch('sprezz.wellknown.PersistentRSAKey.verify_message',
           return_value=True)
    def test_zot_info_hash_not_found(self, rsa_mock):
        context = testing.DummyResource()
        zot = self._makeResourceTree(context)
        request = testing.DummyRequest(params={'guid_hash': 'hash',
                                               'target': 'target',
                                               'target_sig': 'sig',
                                               'key': 'key'},
                                       method='POST')
        request.graph = None
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertFalse(result['success'])
        self.assertEquals(result['message'],
                          'Item not found.')

    @patch('sprezz.wellknown.PersistentRSAKey.verify_message',
           return_value=True)
    def test_zot_info_address_not_found(self, rsa_mock):
        context = testing.DummyResource()
        zot = self._makeResourceTree(context)
        request = testing.DummyRequest(params={'address': 'admin',
                                               'target': 'target',
                                               'target_sig': 'sig',
                                               'key': 'key'},
                                       method='POST')
        request.graph = None
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertFalse(result['success'])
        self.assertEquals(result['message'],
                          'Item not found.')

    @patch('sprezz.wellknown.PersistentRSAKey.verify_message',
           return_value=True)
    def test_zot_info_guid_not_found(self, rsa_mock):
        context = testing.DummyResource()
        zot = self._makeResourceTree(context)
        request = testing.DummyRequest(params={'guid': 'guid',
                                               'guid_sig': 'guid_sig',
                                               'target': 'target',
                                               'target_sig': 'sig',
                                               'key': 'key'},
                                       method='POST')
        request.graph = None
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertFalse(result['success'])
        self.assertEquals(result['message'],
                          'Item not found.')

    @patch('sprezz.wellknown.PersistentRSAKey.verify_message',
           return_value=True)
    def test_zot_info_invalid(self, rsa_mock):
        context = testing.DummyResource()
        zot = self._makeResourceTree(context)
        request = testing.DummyRequest(params={'target': 'target',
                                               'target_sig': 'sig',
                                               'key': 'key'},
                                       method='POST')
        request.graph = None
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertFalse(result['success'])
        self.assertEquals(result['message'],
                          'Invalid request.')
