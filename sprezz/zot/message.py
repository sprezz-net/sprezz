import logging

from zope.interface import implementer

from ..content import content
from ..interfaces import IZotXChannel, IMessage
from ..util.zot import create_channel_hash


log = logging.getLogger(__name__)


@content('Message')
@implementer(IMessage)
class Message(object):
    def __init__(self, sender, data, recipients=None, **kw):
        self.sender = sender
        self.recipients = recipients
        if not IZotXChannel.providedBy(sender):
            message = 'Wrong sender type'
            log.error('Message: {}.'.format(message))
            raise TypeError(message)

        self.message_type = data.get('type', None)
        self.message_id = data.get('message_id', None)

        if 'author' in data:
            self.author_hash = create_channel_hash(data['author']['guid'],
                                                   data['author']['guid_sig'])
        else:
            self.author_hash = None

        if 'owner' in data:
            self.owner_hash = create_channel_hash(data['owner']['guid'],
                                                  data['owner']['guid_sig'])
        else:
            self.owner_hash = None

        self.title = data.get('title', '')
        self.body = data.get('body', '')
        self.mimetype = data.get('mimetype', 'text/bbcode')

        self.flags = data.get('flags', {})

        # FIXME Eventually remove the data record.
        # For now it is kept for access to data which is not
        # yet available as attribute.
        self.data = data

    def check_sender(self):
        return self.sender.channel_hash in [self.author_hash, self.owner_hash]
