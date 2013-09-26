import logging

from persistent import Persistent
from pyramid.traversal import resource_path

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
