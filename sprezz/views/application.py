import json

from logging import getLogger

from aiohttp import web


log = getLogger(__name__)


class ApplicationListView(web.View):
    async def get(self):
        if self.request.can_read_body:
            data = await self.request.json()
            parsed = json.loads(data)
            dump = json.dumps(parsed, indent=2)
            log.info('JSON body: %s', dump)
        return web.Response(text="Welcome")
