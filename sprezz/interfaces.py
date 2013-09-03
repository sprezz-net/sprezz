from zope.interface.interfaces import IObjectEvent
from zope.interface import Interface, Attribute


marker = object()


# TODO Get descriptions from repoze.folder
class IFolder(Interface):
    """A folder which stores objects using Unicode keys."""

    order = Attribute("""Order of items within the folder.""")

    def keys():
        """Return an iterable sequence of object names present in the
        folder."""

    def __iter__():
        """An alias for ``keys``"""

    def values():
        """Return an iterable sequence of the values present in the folder."""

    def items():
        """Return an iterable sequence of (name, value) pairs in the folder."""

    def get(name, default=None):
        """Return the object named by ``name`` or the default."""

    def __contains__(name):
        """Does the container contain an object named by name?"""

    def __nonzero__():
        """Always return True"""

    def __len__():
        """Return the number of subobjects in this folder."""

    def __setitem__(name, other):
        """Set object ``other`` into this folder under the name ``name``."""

    def add(name, other, send_events=True):
        """Same as ``__setitem__``."""

    def pop(name, default=marker):
        """Remove the item stored under ``name`` and return it."""

    def __delitem__(name):
        """Remove the object from this folder stored under ``name``."""

    def remove(name, send_events=True):
        """Same thing as ``__delitem__``."""


class IObjectWillBeAddedEvent(IObjectEvent):
    """An event type sent before an object is added."""
    object = Attribute('The object being added')
    parent = Attribute('The folder to which the object is being added')
    name = Attribute('The name under which the object is being added to '
                     'the folder')


class IObjectAddedEvent(IObjectEvent):
    """An event type sent when an object is added."""
    object = Attribute('The added object')
    parent = Attribute('The folder to which the object is added')
    name = Attribute('The name under which the object is added to '
                     'the folder')

    
class IObjectWillBeRemovedEvent(IObjectEvent):
    """An event type sent before an object is removed."""
    object = Attribute('The object being removed')
    parent = Attribute('The folder from which the object is being removed')
    name = Attribute('The name of the object that is being removed from '
                     'the folder')


class IObjectRemovedEvent(IObjectEvent):
    """An event type sent when an object is removed."""
    object = Attribute('The removed object')
    parent = Attribute('The folder from which the object is removed')
    name = Attribute('The name of the object that is removed from '
                     'the folder')
