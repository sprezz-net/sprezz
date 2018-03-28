from datetime import datetime
from typing import List

from gino import GinoConnection
from oauthlib.common import (generate_client_id,
                             UNICODE_ASCII_CHARACTER_SET)

from sprezz.models import (Client, ClientRedirect,
                           ClientType, GrantType)


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
            client_secret = generate_client_id(length=128,
                                               chars=UNICODE_ASCII_CHARACTER_SET)
        self.client = await Client.create(bind=self.connection,
                                          name=name, id=client_id,
                                          secret=client_secret,
                                          client_type=client_type,
                                          grant_type=grant_type,
                                          owner_id=owner_id)
        for uri in redirect_uri:
            await ClientRedirect.create(bind=self.connection,
                                        client_id=self.client.id,
                                        redirect_uri=uri)
        return self.client

    def iterate_all_clients(self):
        return self.connection.iterate(Client.query)

    def iterate_client_redirect(self, client_id: str) -> ClientRedirect:
        return self.connection.iterate(
            ClientRedirect.query.where(ClientRedirect.client_id == client_id))
