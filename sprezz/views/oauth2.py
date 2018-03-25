import json
import logging

from aiohttp import web


log = logging.getLogger(__name__)


class AuthorizeView(web.View):
    async def get(self):
        log.info('Requested URL: %s', self.request.path_qs)
        log.info('Requested Remote: %s', self.request.remote)
        if self.request.can_read_body:
            data = await self.request.json()
            parsed = json.loads(data)
            dump = json.dumps(parsed, indent=2)
            log.info('JSON body: %s', dump)
        return web.Response(text="Welcome")


class TokenView(web.View):
    async def get(self):
        log.info('Requested URL: %s', self.request.path_qs)
        log.info('Requested Remote: %s', self.request.remote)
        if self.request.can_read_body:
            data = await self.request.json()
            parsed = json.loads(data)
            dump = json.dumps(parsed, indent=2)
            log.info('JSON body: %s', dump)
        return web.Response(text="Welcome")
