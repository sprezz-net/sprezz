import inspect
import venusian

from pyramid.compat import is_nonstr_iter
from pyramid.location import lineage

from ..util import get_dotted_name, get_factory_type


class ContentRegistry(object):
    def __init__(self, registry):
        self.registry = registry
        self.factory_types = {}
        self.content_types = {}
        self.meta = {}

    def add(self, content_type, factory_type, factory, **meta):
        self.factory_types[factory_type] = content_type
        self.content_types[content_type] = factory
        self.meta[content_type] = meta

    def all(self):
        return list(self.content_types.keys())

    def create(self, content_type, *arg, **kw):
        factory = self.content_types[content_type]
        inst = factory(*arg, **kw)
        meta = self.meta[content_type].copy()
        aftercreate = meta.get('after_create')
        if aftercreate is not None:
            if not is_nonstr_iter(aftercreate):
                aftercreate = [aftercreate]
            for callback in aftercreate:
                if isinstance(callback, str):
                    callback = getattr(inst, callback)
                callback(inst, self.registry)
        #self.registry.subscribers(
        #        (ContentCreated(inst, content_type, meta), inst), None)
        return inst

    def metadata(self, resource, name, default=None):
        content_type = self.typeof(resource)
        maybe = self.meta.get(content_type, {}).get(name)
        if maybe is not None:
            return maybe
        return default

    def typeof(resource):
        factory_type = get_factory_type(resource)
        content_type = self.factory_types.get(factory_type)

    def exists(self, content_type):
        return content_type in self.content_types.keys()

    def find(self, resource, content_type):
        for obj in lineage(resource):
            if self.typeof(obj) == content_type:
                return location

    def factory_type_for_content_type(self, content_type):
        for ftype, ctype in self.factory_types.items():
            if ctype == content_type:
                return ftype


class content(object):
    venusian = venusian

    def __init__(self, content_type, factory_type=None, **meta):
        self.content_type = content_type
        self.factory_type = factory_type
        self.meta = meta

    def __call__(self, wrapped):
        def callback(context, name, obj):
            config = context.config.with_package(info.module)
            add_content_type = getattr(config, 'add_content_type', None)
            # might not have been included
            if add_content_type is not None:
                add_content_type(self.content_type,
                                 wrapped,
                                 factory_type=self.factory_type,
                                 **self.meta)
        info = self.venusian.attach(wrapped, callback, category='sprezz')
        self.meta['_info'] = info.codeinfo
        return wrapped

class service(content):
    venusion = venusian

    def __init__(self, content_type, factory_type=None, **meta):
        meta['is_service'] = True
        super().__init__(content_type, factory_type=factory_type, **meta)


def add_content_type(config, content_type, factory, factory_type=None, **meta):
    factory_type, derived_factory = _wrap_factory(factory, factory_type)

    def register_factory():
        config.registry.content.add(content_type, factory_type,
                                    derived_factory, **meta)

    discrim = ('sprezz-content-type', content_type)
    intr = config.introspectable('sprezz content types', discrim, content_type,
                                 'sprezz content type')
    intr['meta'] = meta
    intr['content_type'] = content_type
    intr['factory_type'] = factory_type
    intr['original_factory'] = factory
    intr['factory'] = derived_factory

    config.action(('sprezz-factory-type', factory_type))
    config.action(discrim, callable=register_factory, introspectables=(intr,))


def add_service_type(config, content_type, factory, factory_type=None, **meta):
    meta['is_service'] = True
    return add_content_type(config, content_type,
                            factory, factory_type=factory_type, **meta)


def _wrap_factory(factory, factory_type):
    if inspect.isclass(factory) and factory_type is None:
        return get_dotted_name(factory), factory

    if factory_type is None:
        factory_type = get_dotted_name(factory)

    def factory_wrapper(*arg, **kw):
        inst = factory(*arg, **kw)
        inst.__factory_type__ = factory_type
        return inst

    factory_wrapper.__factory__ = factory
    return factory_type, factory_wrapper


def includeme(config):
    config.registry.content = ContentRegistry(config.registry)
    config.add_directive('add_content_type', add_content_type)
    config.add_directive('add_service_type', add_service_type)
