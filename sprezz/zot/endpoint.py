import json
import logging

from persistent import Persistent
from pprint import pformat
from pyramid.traversal import resource_path
from pyramid.view import view_config

from ..content import content


log = logging.getLogger(__name__)


@content('ZotEndpoint')
class ZotEndpoint(Persistent):
    def get_callback_path(self):
        return resource_path(self)

    def __getitem__(self, key):
        # TODO check if a channel with nick name key exists
        if True:
            dispatch = ZotMagicAuth()
            dispatch.__parent__ = self
            dispatch.__name__ = key
            return dispatch
        raise KeyError(key)


class ZotMagicAuth(object):
    pass


class ZotEndpointView(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request
        self.graph = request.graph

    @view_config(context=ZotEndpoint,
                 renderer='json')
    def post(self):
        log.debug('post: request params = {}'.format(
            pformat(self.request.params)))
        result = {'success': False}
        data = self.request.params.get('data', {})
        try:
            data = json.loads(data)
        except ValueError:
            log.error('post: No valid JSON data received')
            return result
        log.debug('post: data = {}'.format(pformat(data)))

        result['success'] = False  # True
        log.debug('post: result = {}'.format(pformat(result)))
        return result

    @view_config(context=ZotMagicAuth,
                 renderer='json')
    def magic_auth(self):
        log.debug('magic_auth: request params = {}'.format(
            pformat(self.request.params)))
        return {'project': 'magic auth'}
