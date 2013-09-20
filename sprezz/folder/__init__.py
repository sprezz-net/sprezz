import BTrees

from BTrees.Length import Length
from persistent import Persistent
from persistent.interfaces import IPersistent
from pyramid.compat import string_types
from pyramid.location import inside
from pyramid.threadlocal import get_current_registry
from zope.copy.interfaces import ICopyHook, ResumeCopy
from zope.interface import implementer

from ..content import content
from .events import (
    ObjectWillBeAdded,
    ObjectAdded,
    ObjectWillBeRemoved,
    ObjectRemoved
    )
from ..interfaces import IFolder, marker
from ..util.folder import find_service, find_services


@content('Folder')
@implementer(IFolder)
class Folder(Persistent):
    """A folder implementation which acts much like a Python dictionary."""
    family = BTrees.family64

    __name__ = None
    __parent__ = None

    _order = None

    def __init__(self, data=None, family=None):
        """Constructor. Data may be an initial dictionary mapping object name
        to object."""
        super().__init__()
        if family is not None:
            self.family = family
        if data is None:
            data = {}
        self.data = self.family.OO.BTree(data)
        self._num_objects = Length(len(data))

    def keys(self):
        if self._order is not None:
            return self._order
        return self.data.keys()

    def set_order(self, names):
        nameset = set(names)

        if len(self) != len(nameset):
            raise ValueError('Must specify all names when calling set_order')
        if len(names) != len(nameset):
            raise ValueError('No repeated items allowed in names')

        order = []

        for name in names:
            assert(isinstance(name, string_types))
            order.append(name)

        self._order = tuple(order)

    def unset_order(self):
        if self._order is not None:
            del self._order

    order = property(keys, set_order, unset_order)

    def is_ordered(self):
        return self._order is not None

    def __iter__(self):
        return iter(self.keys())

    def values(self):
        if self._order is not None:
            return [self.data[name] for name in self.keys()]
        return self.data.values()

    def items(self):
        if self._order is not None:
            return [(name, self.data[name]) for name in self.keys()]
        return self.data.items()

    def __len__(self):
        return self._num_objects

    def __nonzero__(self):
        return True

    __bool__ = __nonzero__

    def __repr__(self):
        klass = self.__class__
        classname = '%s.%s' % (klass.__module__, klass.__name__)
        return '<%s object named %r at %#x>' % (classname,
                                                self.__name__,
                                                id(self))

    def __getitem__(self, name):
        return self.data[name]

    def get(self, name, default=None):
        return self.data.get(name, default)

    def __contains__(self, name):
        return name in self.data

    def __setitem__(self, name, other):
        return self.add(name, other)

    def validate_name(self, name, reserved_names=()):
        if not isinstance(name, str):
            raise ValueError('Name must be a string rather than a %s' %
                             name.__class__.__name__)
        if not name:
            raise ValueError('Name must not be empty')
        if name in reserved_names:
            raise ValueError('%s is a reserved name' % name)
        if name.startswith('@@'):
            raise ValueError('Names which start with "@@" are not allowed')
        if name.startswith('#'):
            raise ValueError('Names which start with "#" are not allowed')
        if '/' in name:
            raise ValueError('Names which contain a slash ("/") are '
                             'not allowed')
        if '&' in name:
            raise ValueError('Names which contain an ampersand ("&") are '
                             'not allowed')
        return name

    def check_name(self, name, reserved_names=()):
        name = self.validate_name(name, reserved_names=reserved_names)
        if name in self.data:
            raise KeyError('An object named %s already exists' % name)
        return name

    def add(self, name, other, send_events=True, reserved_names=(),
            registry=None):
        if registry is None:
            registry = get_current_registry()

        name = self.check_name(name, reserved_names)

        if getattr(other, '__parent__', None):
            raise ValueError('Object %s added to folder %s already has '
                             'a __parent__ attribute. Please remove it '
                             'completely from its existing parent %s '
                             'before trying to readd it to this one' % (
                                 other, self, self.__parent__))

        if send_events:
            event = ObjectWillBeAdded(other, self, name)
            self._notify(event, registry)

        other.__parent__ = self
        other.__name__ = name

        self.data[name] = other
        self._num_objects.change(1)

        if self._order is not None:
            self._order += (name,)

        if send_events:
            event = ObjectAdded(other, self, name)
            self._notify(event, registry)

        return name

    def pop(self, name, default=marker, registry=None):
        if registry is None:
            registry = get_current_registry()
        try:
            result = self.remove(name, registry=registry)
        except KeyError:
            if default is marker:
                raise
            return default
        return result

    def __delitem__(self, name):
        return self.remove(name)

    def remove(self, name, send_events=True, registry=None):
        if registry is None:
            registry = get_current_registry()

        other = self.data[name]

        if send_events:
            event = ObjectWillBeRemoved(other, self, name)
            self._notify(event, registry)

        if hasattr(other, '__parent__'):
            del other.__parent__
        if hasattr(other, '__name__'):
            del other.__name__

        del self.data[name]
        self._num_objects.change(-1)

        if self._order is not None:
            idx = self._order.index(name)
            order = list(self._order)
            order.pop(idx)
            self._order = tuple(order)

        if send_events:
            event = ObjectRemoved(other, self, name)
            self._notify(event, registry)

        return other

    def _notify(self, event, registry=None):
        if registry is None:
            registry = get_current_registry()
        registry.subscribers((event, event.object, self), None)
        #registry.notify(event)

    def add_service(self, name, obj, registry=None, **kw):
        if registry is None:
            registry = get_current_registry()
        kw['registry'] = registry
        self.add(name, obj, **kw)
        obj.__is_service__ = True

    def find_service(self, name, *subnames):
        return find_service(self, name, subnames)

    def find_services(self, name, *subnames):
        return find_services(self, name, subnames)


def CopyHook(object):
    def __init__(self, context):
        self.context = context

    def __call__(self, toplevel, register):
        context = self.context
        if hasattr(context, '__parent__'):
            if not inside(self.context, toplevel):
                return context
        raise ResumeCopy


def includeme(config):
    config.registry.registerAdapter(CopyHook, (IPersistent,), ICopyHook)
    config.hook_zca()
