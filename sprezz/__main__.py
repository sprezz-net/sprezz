"""Start Sprezz."""
import argparse
import logging
import sys

from asyncio import get_event_loop, set_event_loop_policy, AbstractEventLoop
from typing import List

from aiohttp.web import Application, run_app
from trafaret_config import commandline

from sprezz.models import db
from sprezz.remotes import init_remotes
from sprezz.routes import setup_routes
from sprezz.utils.config import TRAFARET


log = logging.getLogger(__name__)


def attempt_use_uvloop() -> None:
    """Attempt to use uvloop."""
    try:
        import uvloop
        set_event_loop_policy(uvloop.EventLoopPolicy())
    except ImportError:
        pass


def init(loop: AbstractEventLoop, argv: List[str]) -> Application:
    """Initialize Sprezz application."""
    arg_parser = argparse.ArgumentParser()
    commandline.standard_argparse_options(
        arg_parser,
        default_config='./config/sprezz.yaml')
    options = arg_parser.parse_args(argv)
    config = commandline.config_from_options(options, TRAFARET)
    app = Application(loop=loop, middlewares=[db])
    app['config'] = config
    app.on_startup.append(init_remotes)
    db.init_app(app)
    setup_routes(app)
    return app


def main(argv: List[str] = None) -> None:
    """Start Sprezz."""
    if argv is None:
        argv = sys.argv[1:]
    logging.basicConfig(level=logging.DEBUG)
    attempt_use_uvloop()
    loop = get_event_loop()
    app = init(loop, argv)
    run_app(app,
            host=app['config']['listen'],
            port=app['config']['listen_port'])


if __name__ == '__main__':
    main()
