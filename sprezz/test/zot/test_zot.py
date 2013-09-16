import logging
import pprint
import sys
import unittest

from pyramid import testing
from requests.exceptions import HTTPError
from requests.models import Response
from unittest.mock import patch, call, Mock

from ..testing import (
        create_single_content_registry,
        create_dict_content_registry,
        DummyFolder
        )


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
        from sprezz.zot.zot import Zot
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
        registry = create_single_content_registry(ob)
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

    def test_public_site_key(self):
        """Test read-only property key"""
        inst = self._makeOne()
        inst._public_site_key = 'key'
        self.assertEquals(inst.public_site_key, 'key')
        with self.assertRaises(AttributeError):
            inst.public_site_key = 'public'

    def test_site_url(self):
        """Test read-only property site_url and its caching"""
        inst = self._makeOne()
        root = testing.DummyResource()
        root.app_url = 'app_url'
        inst.__parent__ = root
        self.assertEqual(inst.site_url, 'app_url')
        with self.assertRaises(AttributeError):
            inst.site_url = 'new_url'
        self.assertEqual(inst._v_site_url, 'app_url')
        inst._v_site_url = 'cached'
        self.assertEqual(inst.site_url, 'cached')

    def test_site_signature(self):
        """Test read-only property site_signature and its caching"""
        inst = self._makeOne()
        root = testing.DummyResource()
        root.app_url = 'app_url'
        inst.__parent__ = root
        def sign(value):
            return bytes('signed %s' % value, 'utf-8')
        inst._private_site_key = Mock()
        inst._private_site_key.sign_message = Mock(side_effect=sign)
        # Base64 url encoded 'signed app_url'
        self.assertEqual(inst.site_signature, 'c2lnbmVkIGFwcF91cmw')
        with self.assertRaises(AttributeError):
            inst.site_signature = 'new sig'
        self.assertEqual(inst._v_site_signature, 'c2lnbmVkIGFwcF91cmw')
        inst._v_site_signature = 'cached'
        self.assertEqual(inst.site_signature, 'cached')

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
        registry = create_dict_content_registry({'ZotLocalChannel': ob_chan,
                                                 'ZotLocalXChannel': ob_xchan,
                                                 'ZotLocalHub': ob_hub})
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

    @patch('random.randint', return_value=100)
    def test_create_channel_guid(self, rand_mock):
        inst = self._makeOne()
        root = testing.DummyResource()
        root.app_url = 'app_url'
        inst.__parent__ = root
        guid = inst._create_channel_guid('admin')
        # Base64 url encoded Whirlpool hashed 'app_url/admin.100'
        self.assertEqual(guid, 't7towhe6RbhPjwPSTbQMCadsRUHAkUL86F89AGvIoui'
                               '3DIOjA7mNUkMwVzLB6L9-D96VbwXwMp6LMnw2TrjAFg')

    def test_create_channel_signature(self):
        inst = self._makeOne()
        def sign(value):
            return bytes('signed %s' % value, 'utf-8')
        key = Mock()
        key.sign_message = Mock(side_effect=sign)
        sig = inst._create_channel_signature('guid', key)
        # Base64 url encoded 'signed guid'
        self.assertEqual(sig, 'c2lnbmVkIGd1aWQ')

    def test_create_channel_hash(self):
        inst = self._makeOne()
        hashed = inst._create_channel_hash('guid', 'signature')
        # Base64 url encoded Whirlpool hashed 'guidsignature'
        self.assertEqual(hashed, 'EtDtUr7cPtjoXQjDq_d68-AIFRNetS-KudlogIffsYy'
                                 'kH66_LwsOKee3dtoV7KDg1V08s_6K4TVIgo2O5KvNiQ')

    def test_local_empty_zot_finger(self):
        """Test finger with invalid address"""
        inst = self._makeOne()
        root = testing.DummyResource()
        root.netloc = 'netloc:8080'
        inst.__parent__ = root
        with self.assertRaises(ValueError):
            inst.zot_finger('')

    def _prepare_zot_finger(self):
        inst = self._makeOne()
        root = testing.DummyResource()
        root.netloc = 'netloc:8080'
        inst.__parent__ = root
        inst['xchannel'] = DummyFolder()
        inst['hub'] = DummyFolder()

        my_xchannel = testing.DummyResource()
        my_xchannel.address = 'me@netloc:8080'
        my_xchannel.channel_hash = 'me_hash'
        my_xchannel.guid = 'me_guid'
        my_xchannel.signature = 'me_signature'
        my_xchannel.key = Mock()
        my_xchannel.key.export_public_key = Mock(return_value='me_key')
        inst['xchannel'].add(my_xchannel.channel_hash, my_xchannel)

        local_xchannel = testing.DummyResource()
        local_xchannel.address = 'admin@netloc:8080'
        local_xchannel.channel_hash = 'admin_hash'
        inst['xchannel'].add(local_xchannel.channel_hash, local_xchannel)
        local_hub = testing.DummyResource()
        local_hub.url = 'http://hubloc:8080/app_url'
        inst['hub'].add(local_xchannel.channel_hash, local_hub)

        remote_xchannel = testing.DummyResource()
        remote_xchannel.address = 'remote@remoteloc:6543'
        remote_xchannel.channel_hash = 'remote_hash'
        inst['xchannel'].add(remote_xchannel.channel_hash, remote_xchannel)
        remote_hub = testing.DummyResource()
        remote_hub.url = 'https://remoteloc:6543/app_url'
        inst['hub'].add(remote_xchannel.channel_hash, remote_hub)
        return inst

    @patch('sprezz.zot.zot.requests.get')
    def test_local_get_zot_finger(self, req_get_mock):
        """Test succesful finger using HTTP GET"""
        inst = self._prepare_zot_finger()
        resp = Response()
        resp.status_code = 200
        resp.json = Mock(return_value={'success': True})
        req_get_mock.return_value=resp
        result = inst.zot_finger('admin')
        calls = [call('http://hubloc:8080/.well-known/zot-info?address=admin',
                      data=None,
                      timeout=3, verify=True, allow_redirects=True)]
        req_get_mock.assert_has_calls(calls)
        self.assertTrue(result['success'])

    @patch('sprezz.zot.zot.requests.post')
    def test_local_post_zot_finger(self, req_post_mock):
        """Test succesful remote finger from own channel"""
        inst = self._prepare_zot_finger()
        resp = Response()
        resp.status_code = 200
        resp.json = Mock(return_value={'success': True})
        req_post_mock.return_value=resp
        result = inst.zot_finger('admin', 'me_hash')
        calls = [call('http://hubloc:8080/.well-known/zot-info',
                      data={'address': 'admin', 'key': 'me_key',
                            'target': 'me_guid',
                            'target_sig': 'me_signature'},
                      timeout=3, verify=True, allow_redirects=True)]
        req_post_mock.assert_has_calls(calls)
        self.assertTrue(result['success'])

    @patch('sprezz.zot.zot.requests.post')
    def test_local_post_zot_finger_item_not_found(self, req_post_mock):
        """Test unknown remote finger from own channel"""
        inst = self._prepare_zot_finger()
        resp = Response()
        resp.status_code = 200
        resp.json = Mock(return_value={'success': False,
                                       'message': 'Item not found.'})
        req_post_mock.return_value=resp
        with self.assertRaises(ValueError):
            result = inst.zot_finger('admin', 'me_hash')
            calls = [call('http://hubloc:8080/.well-known/zot-info',
                          data={'address': 'admin', 'key': 'me_key',
                                'target': 'me_guid',
                                'target_sig': 'me_signature'},
                          timeout=3, verify=True, allow_redirects=True)]
            req_post_mock.assert_has_calls(calls)

    @patch('sprezz.zot.zot.requests.post')
    def test_local_post_zot_finger_invalid(self, req_post_mock):
        """Test invalid remote finger from own channel"""
        inst = self._prepare_zot_finger()
        resp = Response()
        resp.status_code = 200
        resp.json = Mock(return_value={'success': False})
        req_post_mock.return_value=resp
        with self.assertRaises(ValueError):
            result = inst.zot_finger('admin', 'me_hash')
            calls = [call('http://hubloc:8080/.well-known/zot-info',
                          data={'address': 'admin', 'key': 'me_key',
                                'target': 'me_guid',
                                'target_sig': 'me_signature'},
                          timeout=3, verify=True, allow_redirects=True)]
            req_post_mock.assert_has_calls(calls)

    @patch('sprezz.zot.zot.requests.post')
    def test_remote_post_zot_finger_fallback(self, req_post_mock):
        """Test HTTPS with fallback to HTTP. HTTPS fails with 404, fallback
        succeeds"""
        inst = self._prepare_zot_finger()
        def response_side_effect(url, **kw):
            resp = Response()
            if url.startswith('https'):
                resp.status_code = 404
                resp.reason = 'Not Found'
            else:
                resp.status_code = 200
                resp.json = Mock(return_value={'success': True})
            return resp
        req_post_mock.side_effect = response_side_effect
        result = inst.zot_finger('remote@remoteloc:6543', 'me_hash')
        calls = [call('https://remoteloc:6543/.well-known/zot-info',
                      data={'address': 'remote', 'key': 'me_key',
                            'target': 'me_guid',
                            'target_sig': 'me_signature'},
                      timeout=3, verify=True, allow_redirects=True),
                 call('http://remoteloc:6543/.well-known/zot-info',
                      data={'address': 'remote', 'key': 'me_key',
                            'target': 'me_guid',
                            'target_sig': 'me_signature'},
                      timeout=3, verify=True, allow_redirects=True)]
        req_post_mock.assert_has_calls(calls)
        self.assertTrue(result['success'])

    @patch('sprezz.zot.zot.requests.post')
    def test_remote_post_zot_finger_fallback_notfound(self, req_post_mock):
        """Test HTTPS with fallback to HTTP. Both return 404."""
        inst = self._prepare_zot_finger()
        resp = Response()
        resp.status_code = 404
        resp.reason = 'Not Found'
        req_post_mock.return_value=resp
        with self.assertRaises(HTTPError):
            result = inst.zot_finger('remote@remoteloc:6543', 'me_hash')
            calls = [call('https://remoteloc:6543/.well-known/zot-info',
                          data={'address': 'remote', 'key': 'me_key',
                                'target': 'me_guid',
                                'target_sig': 'me_signature'},
                          timeout=3, verify=True, allow_redirects=True),
                     call('http://remoteloc:6543/.well-known/zot-info',
                          data={'address': 'remote', 'key': 'me_key',
                                'target': 'me_guid',
                                'target_sig': 'me_signature'},
                          timeout=3, verify=True, allow_redirects=True)]
            req_post_mock.assert_has_calls(calls)

    @patch('sprezz.zot.zot.requests.post')
    def test_local_post_zot_finger_no_fallback(self, req_post_mock):
        """Test local finger which fails on HTTP, so no fallback"""
        inst = self._prepare_zot_finger()
        resp = Response()
        resp.status_code = 404
        resp.reason = 'Not Found'
        req_post_mock.return_value=resp
        with self.assertRaises(HTTPError):
            result = inst.zot_finger('admin', 'me_hash')
            calls = [call('http://hubloc:8080/.well-known/zot-info',
                          data={'address': 'admin', 'key': 'me_key',
                                'target': 'me_guid',
                                'target_sig': 'me_signature'},
                          timeout=3, verify=True, allow_redirects=True)]
            req_post_mock.assert_has_calls(calls)
            self.assertEqual(req_post_mock.call_count, 1)
