from typing import List

from gino import GinoConnection
from sqlalchemy.sql.expression import and_

from sprezz.models import (Client, ClientRedirect,
                           ClientType, GrantType)
from sprezz.utils.oauth.common import (generate_client_id,
                                       UNICODE_ASCII_CHARACTER_SET)


__all__ = (
    'ClientService',
)


class ClientService:
    def __init__(self,
                 connection: GinoConnection,
                 client: Client = None) -> None:
        self.connection = connection
        self.client = client

    async def get_client(self, client_id: str) -> Client:
        self.client = await self.connection.first(
            Client.query.where(Client.id == client_id))
        return self.client

    async def register_client(self, name: str,
                              client_type: ClientType,
                              grant_type: GrantType,
                              owner_id: int,
                              redirect_uri: List[str]) -> Client:
        # TODO Make both lengths configurable
        client_id = generate_client_id(length=40,
                                       chars=UNICODE_ASCII_CHARACTER_SET)
        if client_type is ClientType.PUBLIC:
            client_secret = None
        else:
            client_secret = generate_client_id(
                length=128,
                chars=UNICODE_ASCII_CHARACTER_SET)
        self.client = await Client.create(bind=self.connection,
                                          name=name, id=client_id,
                                          secret=client_secret,
                                          client_type=client_type,
                                          grant_type=grant_type,
                                          owner_id=owner_id)
        i = 0
        for uri in redirect_uri:
            await ClientRedirect.create(bind=self.connection,
                                        client_id=self.client.id,
                                        redirect_uri=uri,
                                        default=(i == 0))
            i += 1
        return self.client

    def validate_client(self):
        return self.client.validate

    async def validate_redirect_uri(self, redirect_uri: str) -> bool:
        return await self.connection.first(ClientRedirect.query.where(and_(
            ClientRedirect.client_id == self.client.id,
            ClientRedirect.redirect_uri == redirect_uri))) is not None

    async def query_all_clients(self) -> Client:
        return await self.connection.all(Client.query)

    def iterate_all_clients(self):
        return self.connection.iterate(Client.query)

    async def query_client_redirect(self, client_id: str) -> ClientRedirect:
        return await self.connection.all(
            ClientRedirect.query.where(ClientRedirect.client_id == client_id))

    def iterate_client_redirect(self, client_id: str) -> ClientRedirect:
        return self.connection.iterate(
            ClientRedirect.query.where(ClientRedirect.client_id == client_id))
