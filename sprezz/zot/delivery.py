import logging

from pyramid.threadlocal import get_current_registry
from zope.interface import implementer

from sprezz.interfaces import IDeliverMessage
from sprezz.util.folder import find_service


log = logging.getLogger(__name__)


@implementer(IDeliverMessage)
class DeliverActivity(object):
    def deliver(self, message, **kw):
        """Deliver activity."""
        result = []
        try:
            registry = kw['request'].registry
        except KeyError:
            registry = get_current_registry()

        if not message.check_sender():
            error_message = 'Sender is not owner or author'
            log.error('deliver_activity: {}.'.format(error_message))
            raise ValueError(error_message)
        zot_service = find_service(message.sender, 'zot')

        # TODO Check if message author and owner are known xchannels.
        # If not call refresh to import them

        # TODO Validate message using Colander, or maybe move this up
        # the chain when creating the message.
        item_service = zot_service['item']
        try:
            item = item_service[message.message_id]
        except KeyError:
            # Either array message does not contain a message_id or
            # no message with given message_id exists.
            # Either way, create a new message object.
            item = registry.content.create('ItemMessage', message=message)
            if item.message_id is None:
                # Create a unique message id during add
                item_service.add(None, item)
                # Change the incoming message array so it can be read
                # from the calling method.
                item['message_id'] = item.message_id
            else:
                item_service.add(item.message_id, item)
            action = 'posted'
        else:
            item.update(message)
            action = 'updated'

        for channel in message.recipients:
            # TODO Add local actions to add owner
            recipient = '{0} <{1}>'.format(channel.name,
                                           channel.address)
            result.append([channel.channel_hash, action, recipient])
        return result


def includeme(config):
    config.registry.registerUtility(DeliverActivity, IDeliverMessage,
                                    name='deliver_activity')
    config.hook_zca()
