from zope.interface import Interface, Attribute
from zope.interface import implementer

from ..content import content
from ..folder import Folder


@content('Messages')
class Messages(Folder):
    pass


class IMessage(Interface):
    message_id = Attribute("Unique message ID")
    body = Attribute("Body of message")
    mimetype = Attribute("Mimetype of body")


@content('TextMessage')
@implementer(IMessage)
class TextMessage(Folder):
    def __init__(self, message_id, body, mimetype='text/markdown'):
        super().__init__()
        self.message_id = message_id
        self.body = body
        self.mimetype = mimetype
