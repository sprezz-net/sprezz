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
    T.Key('host', default='0.0.0.0'): IPAddressString,
    T.Key('port', default=8080): T.Int,
    T.Key('reverse_proxy_hosts', optional=True): T.List(IPAddressNetwork),
    T.Key('dsn'): T.String,
})
