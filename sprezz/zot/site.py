import logging

from persistent import Persistent

from ..content import content
from ..folder import Folder


log = logging.getLogger(__name__)


@content('ZotSites')
class ZotSites(Folder):
    pass


@content('ZotSite')
class ZotSite(Persistent):
    def __init__(self, url, register_policy, access_policy,
                 directory_mode, directory_url,
                 version, admin_email):
        super().__init__()
        self.url = url
        self.register_policy = registrater_policy
        self.access_policy = access_policy
        self.directory_mode = directory_mode
        self.directory_url = directory_url
        self.version = version
        self.admin_email = admin_email
