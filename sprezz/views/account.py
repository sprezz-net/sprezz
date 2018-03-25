import json
import os

from functools import partial
from logging import getLogger

import aiofiles

from aiohttp import web

from sprezz.services import AccountService


log = getLogger(__name__)


HERE = os.path.abspath(os.path.dirname(__file__))


def json_serialize(obj):
    if hasattr(obj, 'to_json'):
        return obj.to_json()
    else:
        raise TypeError('Unable to serialize {!r}'.format(obj))


json_dumps = partial(json.dumps, default=json_serialize)


class AccountListView(web.View):
    async def get(self):
        service = AccountService()
        data = []
        engine = self.request.app['engine']
        async with engine.transaction():
            async for account in service.list_all_accounts():
                data.append(account.to_json())
        return web.json_response(data, dumps=json_dumps)
        # TODO look into streaming collections?
        # https://gist.github.com/jbn/fc90e3ddbc5c60c698d07b3df30004c8


class GetAccountView(web.View):
    async def get(self):
        username = self.request.match_info.get('username')
        service = AccountService()
        account = await service.get_account(username)
        return web.json_response(account, dumps=json_dumps)


class CreateAccountView(web.View):
    async def get(self):
        # TODO For testing purposes only a simple file based template
        async with aiofiles.open(os.path.normpath(HERE + '/../template/'
                                                  'create.html'),
                                 encoding='utf-8') as template:
            body = await template.read()
        return web.Response(body=body, content_type='text/html')

    async def post(self):
        if self.request.can_read_body:
            data = await self.request.post()
            service = AccountService()
            await service.create_account(username=data['username'],
                                         email=data['email'],
                                         password=data['password'])
            return web.Response(text="Account created")
        return web.Response(text="Can't read body")
