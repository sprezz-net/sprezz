from datetime import datetime
from typing import List

from gino import GinoConnection
from oauthlib.common import (generate_client_id,
                             UNICODE_ASCII_CHARACTER_SET)

from sprezz.models import (Application, ApplicationRedirect,
                           ClientType, GrantType)


__all__ = (
    'ApplicationService',
)


class ApplicationService:
    def __init__(self,
                 connection: GinoConnection,
                 application: Application = None) -> None:
        self.connection = connection
        self.application = application

    async def get_application(self, app_id: int) -> Application:
        self.application = await self.connection.first(
            Application.query.where(Application.id == app_id))
        return self.application

    async def register_application(self, name: str,
                                   client_type: ClientType,
                                   grant_type: GrantType,
                                   owner_id: int,
                                   redirect_uri: List[str]) -> Application:
        # TODO Make both lengths configurable
        client_id = generate_client_id(length=40,
                                       chars=UNICODE_ASCII_CHARACTER_SET)
        client_secret = generate_client_id(length=128,
                                           chars=UNICODE_ASCII_CHARACTER_SET)
        self.application = await Application.create(bind=self.connection,
                                                    name=name,
                                                    client_id=client_id,
                                                    client_secret=client_secret,
                                                    client_type=client_type,
                                                    grant_type=grant_type,
                                                    owner_id=owner_id)
        for uri in redirect_uri:
            await ApplicationRedirect.create(bind=self.connection,
                                             app_id=self.application.id,
                                             redirect_uri=uri)
        return self.application

    def iterate_all_applications(self):
        return self.connection.iterate(Application.query)

    def iterate_redirect_uri(self, app_id: int) -> ApplicationRedirect:
        return self.connection.iterate(
            ApplicationRedirect.query.where(
                ApplicationRedirect.app_id == app_id))
