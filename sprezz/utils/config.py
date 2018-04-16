from typing import Union
from ipaddress import (IPv4Address, IPv4Network, IPv6Address, IPv6Network,
                       ip_address, ip_network)

import trafaret as T


def check_string_ip_address(value: str) -> Union[str, T.DataError]:
    """IP Address validator for Trafaret"""
    try:
        ip_address(value)
    except ValueError:
        return T.DataError('value is not a valid IPv4 or IPv6 address')
    return value


def check_ip_address_or_network(value: str) -> Union[IPv4Address,
                                                     IPv4Network,
                                                     IPv6Address,
                                                     IPv6Network,
                                                     T.DataError]:
    """IP address or IP network validator for Trafaret"""
    try:
        return ip_address(value)
    except ValueError:
        try:
            return ip_network(value)
        except ValueError:
            return T.DataError('value is not a '
                               'valid IPv4 or IPv6 address or a '
                               'valid network address')


# pylint: disable=invalid-name
IPAddressString = T.String() & check_string_ip_address
# pylint: disable=invalid-name
IPAddressNetwork = T.String() & check_ip_address_or_network


TRAFARET = T.Dict({
    T.Key('host'): T.String,
    T.Key('port', default=443): T.Int,
    T.Key('listen', default='0.0.0.0'): IPAddressString,
    T.Key('listen_port', default=8080): T.Int,
    T.Key('reverse_proxy_hosts', optional=True): T.List(IPAddressNetwork),
    T.Key('gino'): T.Dict({
        T.Key('driver', default='asyncpg', optional=True): T.String,
        T.Key('host', default='localhost', optional=True): T.String,
        T.Key('port', default=5432, optional=True): T.Int,
        T.Key('user', default='postgres', optional=True): T.String,
        T.Key('password', optional=True): T.String,
        T.Key('database', default='postgres', optional=True): T.String,
        T.Key('pool_min_size', default=5, optional=True): T.Int,
        T.Key('pool_max_size', default=10, optional=True): T.Int
    })
})
