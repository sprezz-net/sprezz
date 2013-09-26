import logging
import pprint

from pyramid.view import view_config

from ..poco import PocoChannel, ZotPoco


log = logging.getLogger(__name__)


class PocoView(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request
        self.graph = request.graph

    @view_config(context=ZotPoco,
                 renderer='json')
    def poco(self):
        log.debug('context = %s' % pprint.pformat(self.context))
        log.debug('graph = %s' % pprint.pformat(self.graph))
        return {'project': 'poco'}

    @view_config(context=PocoChannel,
                 renderer='json')
    def poco_channel(self):
        log.debug('context = %s' % pprint.pformat(self.context))
        log.debug('graph = %s' % pprint.pformat(self.graph))
        return {'project': 'poco channel'}
