import logging

from persistent import Persistent

from ..content import content, service
from ..folder import Folder


log = logging.getLogger(__name__)


@service('WellKnown', service_name='well-known', after_create='after_create')
class WellKnown(Folder):
    def after_create(self, inst, registry):
        zot_info = registry.content.create('ZotInfo')
        self.add('zot-info', zot_info)


@content('ZotInfo')
class ZotInfo(Persistent):
    def __getitem__(self, key):
        # TODO Check if address key exists
        if True:
            dispatch = ZotChannelInfo()
            dispatch.__parent__ = self
            dispatch.__name__ = key
            return dispatch
        raise KeyError(key)


class ZotChannelInfo(object):
    pass
