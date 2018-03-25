from asyncio import sleep

from aiohttp.web import Application
from aiohttp_remotes import XForwardedStrict, setup


async def init_remotes(app: Application) -> None:
    try:
        reverse_proxy_hosts = [app['config']['reverse_proxy_hosts']]
    except KeyError:
        await sleep(0)
    else:
        await setup(app, XForwardedStrict(trusted=reverse_proxy_hosts))
