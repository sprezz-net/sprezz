import logging

from zope.interface import implementer

from sprezz.interfaces import IDeliverMessage
from sprezz.util.folder import find_service


log = logging.getLogger(__name__)


@implementer(IDeliverMessage)
class DeliverActivity(object):
    def deliver(self, sender, message, recipients, **kw):
        """Deliver activity.

        ``sender`` is the sending xchannel.
        ``message`` is an array.
        ``recipients`` is a generator for channels that should receive the
        message.
        """
        result = []
        try:
            registry = kw['request'].registry
        except KeyError:
            registry = get_current_registry()
        zot_service = find_service(sender, 'zot')

        author_hash = zot_service.create_channel_hash(
            message['author']['guid'], message['author']['guid_sig'])
        owner_hash = zot_service.create_channel_hash(
            message['owner']['guid'], message['owner']['guid_sig'])

        if sender.channel_hash not in [author_hash, owner_hash]:
            error_message = 'Sender is not owner or author'
            log.error('deliver_activity: {}.'.format(error_message))
            raise ValueError(error_message)

        # TODO Check if message author and owner are known xchannels.
        # If not call refresh to import them

        # TODO Validate message using Colander
        message_service = zot_service['message']
        try:
            item = message_service[message['message_id']]
        except KeyError:
            item = registry.content.create('TextMessage', data=message)
            message_service.add(message['message_id'], item)
            action = 'posted'
        else:
            item.update(message)
            action = 'updated'

        for channel in recipients:
            # TODO Add local actions to add owner
            recipient = '{0} <{1}>'.format(channel.name,
                                           channel.address)
            result.append([channel.channel_hash, action, recipient])
        return result


def includeme(config):
    config.registry.registerUtility(DeliverActivity, IDeliverMessage,
                                    name='deliver_activity')
    config.hook_zca()
