from aiohttp.web import Application
from gino import create_engine

from sprezz.models import db


async def init_database(app: Application) -> None:
    engine = await create_engine(app['config']['dsn'], loop=app.loop)
    app['engine'] = engine
    db.bind = engine
    # await db.gino.drop_all()
    await db.gino.create_all()


async def close_database(app: Application) -> None:
    await app['engine'].close()
