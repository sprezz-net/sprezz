import json
import os

from functools import partial
from logging import getLogger

import aiofiles

from aiohttp import web

from sprezz.models import ClientType, GrantType
from sprezz.services import ClientService


log = getLogger(__name__)


HERE = os.path.abspath(os.path.dirname(__file__))


def json_serialize(obj):
    if hasattr(obj, 'to_json'):
        return obj.to_json()
    else:
        raise TypeError('Unable to serialize {!r}'.format(obj))


json_dumps = partial(json.dumps, default=json_serialize)


class ClientListView(web.View):
    async def get(self):
        data = []
        engine = self.request.app['engine']
        async with engine.acquire() as conn:
            async with conn.transaction() as tx:
                service = ClientService(tx.connection)
                async for client in service.iterate_all_clients():
                    item = client.to_json()
                    item['redirect_uri'] = []
                    async for uri in service.iterate_client_redirect(
                                     client.id):
                        item['redirect_uri'].append(uri.redirect_uri)
                    data.append(item)
        return web.json_response(data, dumps=json_dumps)
        # TODO look into streaming collections?
        # https://gist.github.com/jbn/fc90e3ddbc5c60c698d07b3df30004c8


class RegisterClientView(web.View):
    async def get(self):
        # TODO For testing purposes only a simple file based template
        async with aiofiles.open(os.path.normpath(HERE + '/../template/'
                                                  'register_client.html'),
                                 encoding='utf-8') as template:
            body = await template.read()
        return web.Response(body=body, content_type='text/html')

    async def post(self):
        if self.request.can_read_body:
            data = await self.request.post()
            engine = self.request.app['engine']
            async with engine.acquire() as conn:
                async with conn.transaction() as tx:
                    service = ClientService(tx.connection)
                    client_type = ClientType[data['client_type'].upper()]
                    grant_type = GrantType[data['grant_type'].upper()]
                    redirect_uri = data['redirect_uris'].splitlines()
                    await service.register_client(
                        name=data['name'],
                        client_type=client_type,
                        grant_type=grant_type,
                        owner_id=1,  # TODO Link to real owner
                        redirect_uri=redirect_uri)
            return web.Response(text="Client registered")
        return web.Response(text="Can't read body")
