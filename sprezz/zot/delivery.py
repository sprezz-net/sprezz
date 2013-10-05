import logging

from zope.interface import implementer

from sprezz.interfaces import IDeliverMessage
from sprezz.util.folder import find_service


log = logging.getLogger(__name__)


@implementer(IDeliverMessage)
class DeliverActivity(object):
    def deliver(self, sender, message, recipients):
        zot_service = find_service(self.context, 'zot')
        result = []
        # TODO Message processing
        return result


def includeme(config):
    config.registry.registerUtility(DeliverActivity, IDeliverMessage,
                                    name='deliver_activity')
    config.hook_zca()
