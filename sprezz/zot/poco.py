import logging
import pprint

from persistent import Persistent
from pyramid.traversal import resource_path
from pyramid.view import view_config

from ..content import content


log = logging.getLogger(__name__)


@content('ZotPoco')
class ZotPoco(Persistent):
    def get_callback_path(self):
        return resource_path(self)

    def __getitem__(self, key):
        # TODO check if a channel with nick name key exists
        dispatch = None
        if key == '@me':
            dispatch = PocoMe()
        elif True:
            dispatch = PocoChannel()
        if dispatch is not None:
            dispatch.__parent__ = self
            dispatch.__name__ = key
            return dispatch
        raise KeyError(key)


class PocoChannel(object):
    def __getitem__(self, key):
        if key == '@me':
            dispatch = PocoMe()
            dispatch.__parent__ = self
            dispatch.__name__ = key
            return dispatch
        raise KeyError(key)


class PocoMe(object):
    def __getitem__(self, key):
        dispatch = None
        if key == '@all':
            dispatch = PocoAll()
        elif key == '@self':
            dispatch = PocoSelf()
        if dispatch is not None:
            dispatch.__parent__ = self
            dispatch.__name__ = key
            return dispatch
        raise KeyError(key)


class PocoAll(object):
    pass


class PocoSelf(object):
    pass


class PocoProtocol(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request
        self.graph = request.graph

    @view_config(context=ZotPoco,
                 renderer='json')
    def poco(self):
        log.debug('context = %s' % pprint.pformat(self.context))
        log.debug('graph = %s' % pprint.pformat(self.graph))
        return {'project': 'poco'}

    @view_config(context=PocoChannel,
                 renderer='json')
    def poco_channel(self):
        log.debug('context = %s' % pprint.pformat(self.context))
        log.debug('graph = %s' % pprint.pformat(self.graph))
        return {'project': 'poco channel'}
