import logging
import whirlpool

from Crypto import Random
from pyramid.traversal import find_root
from zope.interface import Interface, Attribute
from zope.interface import implementer

from ..content import content
from ..folder import Folder
from ..util.base64 import base64_url_encode


log = logging.getLogger(__name__)


# Truncate message_id at 64 characters.
MID_SIZE = 64


# Maximum iterations allowed to find an available message_id.
MID_LOOP = 1000


@content('Messages')
class Messages(Folder):
    def generate_id(self, randfunc=None):
        if randfunc is None:
            randfunc = Random.new().read
        root = find_root(self)
        mid = randfunc(MID_SIZE)
        wp = whirlpool.new(mid)
        mid = wp.hexdigest().lower()
        mid = '{}@{}'.format(mid[0:MID_SIZE], root.netloc)
        return mid

    def add(self, message_id, message):
        if message_id is not None:
            return super().add(message_id, message)
        i = 0
        # Prevent infinite loop
        while i < MID_LOOP:
            i += 1
            message_id = self.generate_id()
            try:
                super().add(message_id, message)
            except ValueError:
                continue
            else:
                message.message_id = message_id
                return message_id
        error_message = 'No available message id found'
        log.error('add: {}.'.format(error_message))
        raise KeyError(error_message)


class IMessage(Interface):
    message_id = Attribute("Unique message ID")
    title = Attribute("Title of message")
    body = Attribute("Body of message")
    mimetype = Attribute("Mimetype of body")

    def __init__(self, data):
        """Initialize message with data"""

    def update(self, data):
        """Update message"""


@content('TextMessage')
@implementer(IMessage)
class TextMessage(Folder):
    def __init__(self, data):
        super().__init__()
        self.message_id = data.get('message_id', None)
        self.title = data.get('title', '')
        self.body = data.get('body', '')
        self.mimetype = data.get('mimetype', 'text/bbcode')

    def __json__(self, request):
        return {'message_id': self.message_id,
                'title': self.title,
                'body': self.body,
                'mimetype': self.mimetype}

    def update(self, data):
        keys = ['title', 'body', 'mimetype']
        for x in keys:
            if x in data and getattr(self, x) != data[x]:
                log.debug('message.update() attr={0}, '
                          'old={1}, new={2}'.format(x,
                                                    getattr(self, x),
                                                    data[x]))
                setattr(self, x, data[x])
