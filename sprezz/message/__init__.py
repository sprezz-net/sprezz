import logging

from zope.interface import Interface, Attribute
from zope.interface import implementer

from ..content import content
from ..folder import Folder


log = logging.getLogger(__name__)


@content('Messages')
class Messages(Folder):
    pass


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
        self.message_id = data.get('message_id')
        self.title = data.get('title', '')
        self.body = data.get('body', '')
        self.mimetype = data.get('mimetype', 'text/bbcode')

    def __json__(self, request):
        return {'title': self.title,
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
