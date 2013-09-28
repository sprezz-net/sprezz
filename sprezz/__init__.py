import logging
import transaction

from pyramid.config import Configurator
from pyramid.settings import asbool
from pyramid_zodbconn import get_connection

from .util.translogger import TransLogger


# Adhere to the rules in disutils.version.StrictVersion
__version__ = '0.1a0'


version = __version__
server = 'Sprezz Matrix'


log = logging.getLogger(__name__)


def root_factory(request, t=transaction, g=get_connection):
    conn = g(request)
    zodb_root = conn.root()
    commit = False
    if not 'graph_root' in zodb_root:
        from .models import graphmaker
        zodb_root['graph_root'] = graphmaker()
        t.savepoint()
        commit = True
    request.graph = zodb_root['graph_root']
    if not 'app_root' in zodb_root:
        registry = request.registry
        app_root = registry.content.create('Root')
        zodb_root['app_root'] = app_root
        t.savepoint()
        commit = True
    if commit:
        t.commit()
    return zodb_root['app_root']


def initialize_app_url(scheme, hostname, port, app_path, force_ssl):
    if force_ssl:
        scheme = 'https'
    app_url = '{0}://{1}'.format(scheme, hostname)
    if (scheme == 'http' and port != 80) or (
            scheme == 'https' and port != 443):
        app_url = '{0}:{1}'.format(app_url, port)
    if app_path is not None:
        app_url = '{0}/{1}'.format(app_url, app_path)
    return app_url


def initialize_sprezz(settings):
    """Normalize configuration"""
    def strip_bool(value):
        if value is not None:
            return asbool(value)
        return None

    def strip_int(value):
        if value is not None:
            return int(value)
        return None

    def strip_string(value):
        if value is not None:
            return value.strip()
        return None

    def strip_path(value):
        if value is not None:
            return value.strip().strip('/')
        return None

    hostname = strip_path(settings.get('sprezz.hostname', None))
    if hostname is None:
        log.error("Please set a hostname configuration with key "
                  "'sprezz.hostname'")

    force_ssl = strip_bool(settings.get('sprezz.force_ssl', 'false'))
    if force_ssl:
        scheme = strip_string(settings.get('sprezz.scheme', 'https'))
    else:
        scheme = strip_string(settings.get('sprezz.scheme', 'http'))

    if scheme == 'https':
        port = strip_int(settings.get('sprezz.port', '443'))
    elif scheme == 'http':
        port = strip_int(settings.get('sprezz.port', '80'))
    else:
        log.error("Configuration setting with key 'sprezz.scheme' must be "
                  "one of 'http' or 'https'.")

    app_path = strip_path(settings.get('sprezz.app_path', None))

    settings['sprezz.force_ssl'] = force_ssl
    settings['sprezz.scheme'] = scheme
    settings['sprezz.hostname'] = hostname
    settings['sprezz.port'] = port
    settings['sprezz.app_path'] = app_path
    settings['sprezz.app_url'] = initialize_app_url(scheme, hostname, port,
                                                    app_path, force_ssl)

    admin_channel = strip_string(settings.get('sprezz.admin.channel',
                                              'admin'))
    admin_name = strip_string(settings.get('sprezz.admin.name',
                                           'Administrator'))
    admin_email = strip_string(settings.get('sprezz.admin.email',
                                            '@'.join(['webmaster', hostname])))

    settings['sprezz.admin.channel'] = admin_channel
    settings['sprezz.admin.name'] = admin_name
    settings['sprezz.admin.email'] = admin_email


def include(config):
    config.include('pyramid_chameleon')
    config.include('pyramid_zodbconn')
    config.include('.content')
    config.include('.folder')


def scan(config):
    config.scan('.folder')
    config.scan('.root')
    config.scan('.wellknown')
    config.scan('.zot')
    config.scan('.message')
    config.scan('.views')


def includeme(config):
    config.include(include)
    config.include(scan)
    settings = config.registry.settings
    initialize_sprezz(settings)


def main(global_config, **settings):
    """ This function returns a Pyramid WSGI application.
    """
    config = Configurator(root_factory=root_factory, settings=settings)
    config.include('sprezz')
    config.add_static_view('static', 'static', cache_max_age=3600)
    includeme(config)

    app = config.make_wsgi_app()
    app = TransLogger(app, setup_console_handler=False)
    return app
