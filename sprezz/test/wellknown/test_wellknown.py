import logging
import unittest

from pyramid import testing
from unittest.mock import patch, Mock

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


class TestZotInfoView(unittest.TestCase):
    def setUp(self):
        request = testing.DummyRequest()
        self.config = testing.setUp(request=request)

    def tearDown(self):
        testing.tearDown()

    def _makeOne(self, context, request):
        from sprezz.wellknown.views import ZotInfoView
        return ZotInfoView(context, request)

    def _makeResourceTree(self, context):
        root = DummyFolder()
        root['context'] = context
        zot = root['zot'] = DummyFolder()
        zot.__is_service__ = True
        zot.site_url = 'site_url'
        zot['channel'] = DummyFolder()
        zot['xchannel'] = DummyFolder()
        zot['hub'] = DummyFolder()
        return zot

    def _makeChannels(self, zot):
        from sprezz.zot.channel import (
            ZotLocalChannel,
            ZotRemoteXChannel,
            ZotRemoteHub)

        def sign(value):
            return bytes('signed %s' % value, 'utf-8')

        key = Mock()
        key.sign = Mock(side_effect=sign)
        key.export_public_key = Mock(return_value='key')
        site_key = Mock()
        site_key.export_public_key = Mock(return_value='site_key')
        channel = ZotLocalChannel(nickname='nickname',
                                  name='name',
                                  channel_hash='hash',
                                  guid='guid',
                                  signature='guid_sig',
                                  key=key)
        xchannel = ZotRemoteXChannel(nickname='nickname',
                                     name='name',
                                     channel_hash='hash',
                                     guid='guid',
                                     signature='guid_sig',
                                     key=key,
                                     address='address',
                                     url='url',
                                     connections_url='conn_url',
                                     photo='photo',
                                     flags='flags')
        hub = ZotRemoteHub(channel_hash='hash',
                           guid='guid',
                           signature='guid_sig',
                           key=site_key,
                           host='host',
                           address='hub_address',
                           url='hub_url',
                           url_signature='url_sig',
                           callback='callback')
        zot['channel'].add(channel.nickname, channel)
        zot['xchannel'].add(xchannel.channel_hash, xchannel)
        zot['hub'].add(hub.channel_hash, hub)

    def _assertZotInfo(self, result):
        self.assertEqual(result['guid'], 'guid')
        self.assertEqual(result['guid_sig'], 'guid_sig')
        self.assertEqual(result['key'], 'key')
        self.assertEqual(result['name'], 'name')
        self.assertEqual(result['address'], 'address')
        self.assertEqual(result['url'], 'url')
        self.assertEqual(result['connections_url'], 'conn_url')
        self.assertEqual(result['target'], 'target')
        self.assertEqual(result['target_sig'], 'sig')
        location = result['locations'][0]
        self.assertEqual(location['host'], 'host')
        self.assertEqual(location['address'], 'hub_address')
        self.assertTrue(location['primary'])
        self.assertEqual(location['url'], 'hub_url')
        self.assertEqual(location['url_sig'], 'url_sig')
        self.assertEqual(location['callback'], 'callback')
        self.assertEqual(location['sitekey'], 'site_key')
        site = result['site']
        self.assertEqual(site['url'], 'site_url')
        # Base64 url encoded 'signed site_url'
        self.assertEqual(site['url_sig'], 'c2lnbmVkIHNpdGVfdXJs')
        self.assertEqual(site['admin'], 'email')

    def test_zot_info_no_key(self):
        context = testing.DummyResource()
        request = testing.DummyRequest(params={'target': 'target'})
        request.graph = None
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertFalse(result['success'])
        self.assertEquals(result['message'],
                          'No key or target signature supplied.')

    @patch('sprezz.wellknown.views.PersistentRSAKey.verify',
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

    @patch('sprezz.wellknown.views.PersistentRSAKey.verify',
           return_value=True)
    def test_zot_info_hash_not_found(self, rsa_mock):
        context = testing.DummyResource()
        self._makeResourceTree(context)
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

    @patch('sprezz.wellknown.views.PersistentRSAKey.verify',
           return_value=True)
    def test_zot_info_address_not_found(self, rsa_mock):
        context = testing.DummyResource()
        self._makeResourceTree(context)
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

    @patch('sprezz.wellknown.views.PersistentRSAKey.verify',
           return_value=True)
    def test_zot_info_guid_not_found(self, rsa_mock):
        context = testing.DummyResource()
        self._makeResourceTree(context)
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

    @patch('sprezz.wellknown.views.PersistentRSAKey.verify',
           return_value=True)
    def test_zot_info_invalid(self, rsa_mock):
        context = testing.DummyResource()
        self._makeResourceTree(context)
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

    @patch('sprezz.wellknown.views.PersistentRSAKey.verify',
           return_value=True)
    def test_zot_info_hash_found(self, rsa_mock):
        context = testing.DummyResource()
        zot = self._makeResourceTree(context)
        self._makeChannels(zot)
        request = testing.DummyRequest(params={'guid_hash': 'hash',
                                               'target': 'target',
                                               'target_sig': 'sig',
                                               'key': 'key'},
                                       method='POST')
        request.graph = None
        request.registry = Mock()
        request.registry.settings = {'sprezz.admin.email': 'email'}
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertTrue(result['success'])
        self._assertZotInfo(result)

    @patch('sprezz.wellknown.views.PersistentRSAKey.verify',
           return_value=True)
    def test_zot_info_address_found(self, rsa_mock):
        context = testing.DummyResource()
        zot = self._makeResourceTree(context)
        self._makeChannels(zot)
        request = testing.DummyRequest(params={'address': 'nickname',
                                               'target': 'target',
                                               'target_sig': 'sig',
                                               'key': 'key'},
                                       method='POST')
        request.graph = None
        request.registry = Mock()
        request.registry.settings = {'sprezz.admin.email': 'email'}
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertTrue(result['success'])
        self._assertZotInfo(result)

    @patch('sprezz.wellknown.views.PersistentRSAKey.verify',
           return_value=True)
    def test_zot_info_guid_found(self, rsa_mock):
        context = testing.DummyResource()
        zot = self._makeResourceTree(context)
        self._makeChannels(zot)
        request = testing.DummyRequest(params={'guid': 'guid',
                                               'guid_sig': 'guid_sig',
                                               'target': 'target',
                                               'target_sig': 'sig',
                                               'key': 'key'},
                                       method='POST')
        request.graph = None
        request.registry = Mock()
        request.registry.settings = {'sprezz.admin.email': 'email'}
        inst = self._makeOne(context, request)
        result = inst.zot_info()
        self.assertTrue(result['success'])
        self._assertZotInfo(result)
