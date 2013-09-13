import types

from pyramid.location import lineage

from ..interfaces import IFolder


def get_dotted_name(obj):
    """Return the dotted name of a global object."""
    name = obj.__name__
    if isinstance(obj, types.ModuleType):
        return name
    module = obj.__module__
    return '.'.join((module, name))


def get_factory_type(resource):
    factory_type = getattr(resource, '__factory_type__', None)
    if factory_type is None:
        factory_type = get_dotted_name(resource.__class__)
    return factory_type


def _traverse_to(obj, names):
    for name in names:
        if not is_folder(obj):
            return None
        obj = obj.get(name, None)
        if obj is None:
            return None
    return obj


def _find_services(resource, name, subnames=(), one=False):
    L = []
    for obj in lineage(resource):
        if is_folder(obj):
            subobj = obj.get(name, None)
            if subobj is not None:
                if is_service(subobj):
                    if subnames:
                        subobj = _traverse_to(subobj, subnames)
                        if subobj is None:
                            continue
                    if one:
                        return subobj
                    L.append(subobj)
    if one:
        return None
    return L


def find_service(resource, name, *subnames):
    return _find_services(resource, name, subnames, one=True)


def find_services(resource, name, *subnames):
    return _find_services(resource, name, subnames)


def is_folder(resource):
    return IFolder.providedBy(resource)


def is_service(resource):
    return bool(getattr(resource, '__is_service__', False))
