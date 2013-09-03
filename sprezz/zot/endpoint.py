import logging
import pprint

from persistent import Persistent
from pyramid.traversal import resource_path
from pyramid.view import view_config

from ..content import content
from ..folder import Folder


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


class ZotProtocol(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request
        self.graph = request.graph

    @view_config(context=ZotEndpoint,
                 renderer='json')
    def post(self):
        log.debug('context = %s' % pprint.pformat(self.context))
        log.debug('graph = %s' % pprint.pformat(self.graph))
        return {'project': 'post'}

    @view_config(context=ZotMagicAuth,
                 renderer='json')
    def magic_auth(self):
        log.debug('context = %s' % pprint.pformat(self.context))
        log.debug('graph = %s' % pprint.pformat(self.graph))
        return {'project': 'magic auth'}
