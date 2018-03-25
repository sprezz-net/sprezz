import json

from logging import getLogger

from aiohttp import web


log = getLogger(__name__)


async def catch_all(request):
    log.info('Requested URL: %s', request.path_qs)
    if request.can_read_body:
        data = await request.json()
        parsed = json.loads(data)
        dump = json.dumps(parsed, indent=2)
        log.info('JSON body: %s', dump)
    return web.Response(text="Default catch all handler")
