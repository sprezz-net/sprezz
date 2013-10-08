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


class IZotChannel(Interface):
    """Interface for a Zot channel."""
    nickname = Attribute('Nickname of channel')
    name = Attribute('Channel description')
    channel_hash = Attribute('Hash of channel')
    guid = Attribute('Global unique identifier')
    signature = Attribute('Signature of guid')
    key = Attribute('Public private keypair')

    address = Attribute('Channel address')


class IZotXChannel(Interface):
    """Interface for Zot xchannels."""
    nickname = Attribute('Nickname of channel')
    name = Attribute('Channel description')
    channel_hash = Attribute('Hash of channel')
    guid = Attribute('Global unique identifier')
    signature = Attribute('Signature of guid')
    key = Attribute('Public private keypair')

    photo = Attribute('Channal photo avatar')
    flags = Attribute('Channel flags')

    address = Attribute('Channel address')
    url = Attribute('Channel URL')
    connections_url = Attribute('Connections URL')

    def update(self, data):
        """Update channel with data"""


class IZotHub(Interface):
    """Interface for Zot hubs."""
    channel_hash = Attribute('Hash of channel')
    guid = Attribute('Global unique identifier')
    signature = Attribute('Signature of guid')
    key = Attribute('Public private keypair')

    host = Attribute('Host of hub')
    address = Attribute('Channel address')
    url = Attribute('Hub URL')
    url_signature = Attribute('Signature of hub URL')
    callback = Attribute('Zot endpoint URL')

    def update(self, data):
        """Update hub with data"""


class IZotSite(Interface):
    """Interface for Zot sites."""
    url = Attribute('URL of site')
    register_policy = Attribute('Site registration policy')
    access_policy = Attribute('Site access policy')
    directory_mode = Attribute('Mode to list site in directory')
    directory_url = Attribute('URL of site directory')
    version = Attribute('Version')
    admin_email = Attribute('Email address of site administrator')

    def update(self, data):
        """Update site with data"""


class IPostEndpoint(Interface):
    """Interface to define post endpoint methods."""
    context = Attribute('Context of request')
    request = Attribute('Request')

    def __init__(self, context, request):
        """Initialize endpoint with request data"""

    def post(self, data):
        """Handle request with parameter data"""


class IDeliverMessage(Interface):
    """Interface to define message delivery methods."""
    def deliver(self, sender, message, recipients, **kw):
        """Deliver message from sender to recipients"""


class IMessage(Interface):
    """Interface for messages."""
    message_id = Attribute('Unique message ID')
    title = Attribute('Title of message')
    body = Attribute('Body of message')
    mimetype = Attribute('Mimetype of body')

    def __init__(self, data):
        """Initialize message with data"""

    def update(self, data):
        """Update message"""


class IQueueMessage(Interface):
    """Interface for queued messages"""
