from rdflib import ConjunctiveGraph


def graphmaker():
    graph_root = ConjunctiveGraph('ZODB')
    return graph_root
