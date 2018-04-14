import pathlib

from aiohttp import web

from sprezz.views.account import (AccountListView,
                                  GetAccountView,
                                  CreateAccountView)
from sprezz.views.client import (ClientListView,
                                 RegisterClientView)
from sprezz.views.catchall import catch_all
from sprezz.views.oauth2 import AuthorizeView, TokenView
from sprezz.views import setup_view_routes


def setup_routes(app: web.Application) -> None:
    setup_view_routes(app)

    app.add_routes([web.view('/connect/authorize', AuthorizeView),
                    web.view('/connect/token', TokenView),
                    web.view('/client', ClientListView),
                    web.view('/client/register', RegisterClientView),
                    web.view('/account', AccountListView),
                    web.view('/account/create', CreateAccountView),
                    web.view('/account/{username}', GetAccountView),
                    web.route('*', '/{tail:.*}', catch_all)])

    project_root = pathlib.Path(__file__).parent.parent
    app.add_routes([web.static('/static/',
                               path=project_root / 'static',
                               name='static')])
