import pathlib

from aiohttp import web

from sprezz.views.account import (AccountListView,
                                  GetAccountView,
                                  CreateAccountView)
from sprezz.views.client import (ClientListView,
                                 RegisterClientView)
from sprezz.views.catchall import catch_all
from sprezz.views.oauth2 import AuthorizeView, TokenView


PROJECT_ROOT = pathlib.Path(__file__).parent.parent


def setup_routes(app: web.Application) -> None:
    app.add_routes([web.view('/authorize', AuthorizeView),
                    web.view('/token', TokenView),
                    web.view('/client', ClientListView),
                    web.view('/client/register', RegisterClientView),
                    web.view('/account', AccountListView),
                    web.view('/account/create', CreateAccountView),
                    web.view('/account/{username}', GetAccountView),
                    web.route('*', '/{tail:.*}', catch_all)])
    app.add_routes([web.static('/static/',
                               path=PROJECT_ROOT / 'static',
                               name='static')])
