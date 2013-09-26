import logging

from pyramid.view import view_config

from ..channel import ZotChannels, ZotLocalChannel
from sprezz.util.folder import find_service


log = logging.getLogger(__name__)


class ChannelView(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request
        self.graph = request.graph

    @view_config(context=ZotChannels,
                 request_method='POST',
                 renderer='json')
    def create_channel(self):
        # TODO validate name and nickname (colander)
        nickname = self.request.params['nickname'].strip().lower()
        name = self.request.params['name'].strip()
        zot_service = find_service(self.context, 'zot')
        channel = zot_service.add_channel(nickname, name)
        return channel.__json__(self.request)

    @view_config(context=ZotChannels,
                 request_method='GET',
                 renderer='json')
    def list_channels(self):
        return {'channels': [c.__json__(self.request) for c in
                             self.context.values()]}

    @view_config(context=ZotLocalChannel,
                 request_method='POST',
                 name='connection',
                 renderer='json')
    def add_connection(self):
        result = {'success': False}
        url = self.request.params['url'].strip()
        zot_service = find_service(self.context, 'zot')
        #try:
        info = zot_service.zot_finger(url, self.context.channel_hash)
        #except Exception as e:
            # TODO Check for the various exception classes like HTTP
            # exeception, etc.
            #result['message'] = str(e)
            #return result

        zot_service.import_xchannel(info)

        result['success'] = True
        result['info'] = info
        return result
