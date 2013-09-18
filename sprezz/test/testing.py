import logging

from pyramid import testing
from zope.interface import implementer

from sprezz.interfaces import IFolder


log = logging.getLogger(__name__)


class DummySingleContentRegistry(object):
    def __init__(self, result):
        self.result = result

    def create(self, content_type, *arg, **kw):
        return self.result


class DummyDictContentRegistry(object):
    def __init__(self, result):
        self.result = result

    def create(self, content_type, *arg, **kw):
        return self.result[content_type]


@implementer(IFolder)
class DummyFolder(testing.DummyResource):
    def __init__(self):
        super().__init__()
        self.data = {}

    def add(self, name, val, registry=None):
        val.__name__ = name
        val.__parent__ = self
        self.data[name] = val
        return name

    def get(self, name, default=None):
        return self.data.get(name, default)

    def keys(self):
        return self.data.keys()

    def values(self):
        return self.data.values()

    def __getitem__(self, name):
        return self.data[name]

    def __setitem__(self, name, val):
        return self.add(name, val)

    def __iter__(self):
        return iter(self.keys())


def create_single_content_registry(ob):
    content = DummySingleContentRegistry(ob)
    registry = testing.DummyResource()
    registry.content = content
    return registry


def create_dict_content_registry(d):
    content = DummyDictContentRegistry(d)
    registry = testing.DummyResource()
    registry.content = content
    return registry
