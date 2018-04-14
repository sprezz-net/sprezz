from aiohttp import web

from sprezz.utils.accept import AcceptChooser


chooser = AcceptChooser()  # pylint: disable=invalid-name


@chooser.accept('GET', 'application/json')
async def webfinger_get_json(request):
    data = {'method': 'webfinger'}
    return web.json_response(data)


def setup_routes(app: web.Application) -> None:
    app.add_routes([web.route('*', '/.well-known/webfinger', chooser.route)])
