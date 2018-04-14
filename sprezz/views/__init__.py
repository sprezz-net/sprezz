from aiohttp.web import Application

from . import webfinger


VIEWS = [webfinger, ]


def setup_view_routes(app: Application) -> None:
    for view in VIEWS:
        view.setup_routes(app)
