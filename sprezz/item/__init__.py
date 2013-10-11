import logging
import whirlpool

from Crypto import Random
from pyramid.traversal import find_root
from zope.interface import implementer

from ..content import content
from ..folder import Folder
from ..interfaces import IItem


log = logging.getLogger(__name__)


# Truncate message_id at 64 characters.
MID_SIZE = 64


# Maximum iterations allowed to find an available message_id.
MID_LOOP = 1000


@content('Items')
class Items(Folder):
    def generate_id(self, randfunc=None):
        if randfunc is None:
            randfunc = Random.new().read
        root = find_root(self)
        mid = randfunc(MID_SIZE)
        wp = whirlpool.new(mid)
        mid = wp.hexdigest().lower()
        mid = '{}@{}'.format(mid[0:MID_SIZE], root.netloc)
        return mid

    def add(self, message_id, item):
        if message_id is not None:
            return super().add(message_id, item)
        i = 0
        # Prevent infinite loop
        while i < MID_LOOP:
            i += 1
            message_id = self.generate_id()
            try:
                super().add(message_id, item)
            except ValueError:
                continue
            else:
                if i > 1:
                    log.debug('add: Found available message id after {} '
                              'iterations.'.format(i))
                item.message_id = message_id
                return message_id
        error_message = 'No available message id found'
        log.error('add: {}.'.format(error_message))
        raise KeyError(error_message)


@content('ItemMessage')
@implementer(IItem)
class TextItem(Folder):
    def __init__(self, message):
        super().__init__()
        self.message_id = getattr(message, 'message_id', None)
        self.title = getattr(message, 'title', '')
        self.body = getattr(message, 'body', '')
        self.mimetype = getattr(message, 'mimetype', 'text/bbcode')

    def __json__(self, request):
        return {'message_id': self.message_id,
                'title': self.title,
                'body': self.body,
                'mimetype': self.mimetype}

    def update(self, message):
        keys = ['title', 'body', 'mimetype']
        for x in keys:
            value = getattr(message, x)
            if getattr(self, x) != value:
                log.debug('update: attr={0}, '
                          'old={1}, new={2}'.format(x,
                                                    getattr(self, x),
                                                    value))
                setattr(self, x, value)
