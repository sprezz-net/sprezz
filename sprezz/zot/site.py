import logging

from persistent import Persistent
from zope.interface import implementer

from ..content import content
from ..folder import Folder
from ..interfaces import IZotSite


log = logging.getLogger(__name__)


@content('ZotSites')
class ZotSites(Folder):
    pass


@content('ZotSite')
@implementer(IZotSite)
class ZotSite(Persistent):
    def __init__(self, url, register_policy, access_policy,
                 directory_mode, directory_url,
                 version, admin_email):
        super().__init__()
        self.url = url
        self.register_policy = register_policy
        self.access_policy = access_policy
        self.directory_mode = directory_mode
        self.directory_url = directory_url
        self.version = version
        self.admin_email = admin_email

    def update(self, data):
        keys = ['url', 'register_policy', 'access_policy',
                'directory_mode', 'directory_url', 'version', 'admin']
        for x in keys:
            if (x in data) and (getattr(self, x) != data[x]):
                log.debug('site.update() attr={0}, '
                          'old={1}, new={2}'.format(x,
                                                    getattr(self, x),
                                                    data[x]))
                setattr(self, x, data[x])
