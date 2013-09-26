import logging

from persistent import Persistent
from pyramid.traversal import resource_path

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
