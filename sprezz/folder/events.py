from zope.interface import implementer

from ..interfaces import (
        IObjectWillBeAddedEvent,
        IObjectAddedEvent,
        IObjectWillBeRemovedEvent,
        IObjectRemovedEvent
        )


class _ObjectEvent(object):
    def __init__(self, object, parent, name):
        self.object = object
        self.parent = parent
        self.name = name


@implementer(IObjectWillBeAddedEvent)
class ObjectWillBeAdded(_ObjectEvent):
    """An event sent just before an object has been added to a folder."""


@implementer(IObjectAddedEvent)
class ObjectAdded(_ObjectEvent):
    """An event sent after an object has been added to a folder."""


@implementer(IObjectWillBeRemovedEvent)
class ObjectWillBeRemoved(_ObjectEvent):
    """An event sent just before an object has been removed from a folder."""


@implementer(IObjectRemovedEvent)
class ObjectRemoved(_ObjectEvent):
    """An event sent after an object has been removed from a folder."""
