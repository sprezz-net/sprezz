import logging
import whirlpool

from Crypto import Random
from zope.interface import implementer

from ..content import content
from ..interfaces import IQueueMessage
from ..folder import Folder


log = logging.getLogger(__name__)


SECRET_SIZE = 64


SECRET_LOOP = 1000


@content('Queue', after_create='after_create')
class Queue(Folder):
    def after_create(self, inst, registry):
        outgoing_queue = registry.content.create('OutgoingQueue')
        self.add('outgoing', outgoing_queue)


@content('OutgoingQueue')
class OutgoingQueue(Folder):
    def generate_secret(self, randfunc=None):
        if randfunc is None:
            randfunc = Random.new().read
        secret = randfunc(SECRET_SIZE)
        wp = whirlpool.net(secret)
        secret = wp.hexdigest().lower()
        secret = secret[0:SECRET_SIZE]
        return secret

    def add(self, secret, data):
        if secret is not None:
            return super().add(secret, data)
        i = 0
        # Prevent infinite loop
        while i < SECRET_LOOP:
            i += 1
            secret = self.generate_secret()
            try:
                super().add(secret, data)
            except ValueError:
                continue
            else:
                if i > 1:
                    log.debug('add: Found available secret after {} '
                              'iterations.'.format(i))
                data.secret = secret
                return secret
        error_message = 'No available secret found'
        log.error('add: {}.'.format(error_message))
        raise KeyError(error_message)


@content('QueueMessage')
@implementer(IQueueMessage)
class QueueMessage(Folder):
    def __init__(self, sender, hub, secret=None, data=None):
        super().__init__()
        self.secret = secret
        self.sender = sender
        self.hub = hub
        self.data = data
