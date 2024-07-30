import asyncio
import json
import random

class StratumProxy:
    def __init__(self, config_file):
        self.load_config(config_file)
        self.pool_connections = {}

    def load_config(self, config_file):
        with open(config_file, 'r') as f:
            self.config = json.load(f)
        total_proportion = sum(pool['proportion'] for pool in self.config['pools'])
        self.pool_weights = [(pool['address'], pool['proportion'] / total_proportion) for pool in self.config['pools']]

    def select_pool(self):
        return random.choices([address for address, _ in self.pool_weights], [weight for _, weight in self.pool_weights])[0]

    async def handle_client(self, reader, writer):
        pool_address = self.select_pool()
        pool_reader, pool_writer = await asyncio.open_connection(*pool_address.split(':'))
        # if pool_address not in self.pool_connections:
            # pool_reader, pool_writer = await asyncio.open_connection(*pool_address.split(':'))
            # self.pool_connections[pool_address] = (pool_reader, pool_writer)
        # else:
            # pool_reader, pool_writer = self.pool_connections[pool_address]

        client_to_pool = self.transfer_data(reader, pool_writer)
        pool_to_client = self.transfer_data(pool_reader, writer)

        await asyncio.gather(client_to_pool, pool_to_client)
        
        writer.close()
        await writer.wait_closed()

    async def transfer_data(self, reader, writer):
        try:
            while not reader.at_eof():
                data = await reader.read(1024)
                print(data)
                if not data:
                    break
                writer.write(data)
                await writer.drain()
        except asyncio.CancelledError:
            pass
        except Exception as e:
            print(f"Data transfer error: {e}")
        finally:
            writer.close()

    async def start_server(self, host='0.0.0.0', port=3333):
        server = await asyncio.start_server(self.handle_client, host, port)
        async with server:
            await server.serve_forever()

if __name__ == '__main__':
    proxy = StratumProxy('config.json')
    asyncio.run(proxy.start_server())
