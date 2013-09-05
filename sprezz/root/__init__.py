import logging

from pyramid.threadlocal import get_current_registry

from ..content import content
from ..folder import Folder


log = logging.getLogger(__name__)


@content('Root', after_create='after_create')
class Root(Folder):
    def after_create(self, inst, registry):
        log.info('Registering Root object')
        zot = registry.content.create('Zot')
        self.add_service('zot', zot, registry=registry)
        well_known = registry.content.create('WellKnown')
        self.add_service('.well-known', well_known, registry=registry)
        # TODO
        #channel = zot.add_channel('admin', 'Administrator')
        log.info('Root object created')

    def get_app_url(self, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()
        settings = registry.settings
        return settings['sprezz.app_url']

    def get_host(self, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()
        settings = registry.settings
        return settings['sprezz.hostname']

    def get_port(self, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()
        settings = registry.settings
        return settings['sprezz.port']
    
    app_url = property(get_app_url)
    hostname = property(get_host)
    port = property(get_port)
