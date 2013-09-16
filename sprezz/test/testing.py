from pyramid import testing


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


class DummyFolder(testing.DummyResource):
    def __init__(self):
        super().__init__()
        self.data = {}

    def add(self, name, val, registry=None):
        self.data[name] = val
        return name

    def values(self):
        return self.data.values()

    def __getitem__(self, name):
        return self.data[name]


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
