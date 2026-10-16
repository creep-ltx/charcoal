// Runs Moebius's own collaboration server (app/server.js of blocktronics/moebius)
// for CharEdit's tests, on 127.0.0.1 only (give it a copy of the file: Moebius's
// server writes the page back to it when closed, and hourly backups beside it):
//   node moebius-server.js <moebius app folder> <port> <password> <file> <path>
// NODE_PATH must hold ws (the real one) and stubs for discord.js and upng-js
// (the server needs neither): see README.md.
const http = require("http");
const realCreate = http.createServer;
http.createServer = (...a) => {
    const s = realCreate(...a);
    const listen = s.listen.bind(s);
    s.listen = (port, ...rest) => listen(port, "127.0.0.1", ...rest);
    return s;
};
const [app, port, pass, file, path] = process.argv.slice(2);
const server = require(require("path").resolve(app, "server"));
server.start_joint({path, file, pass, quiet: true, server_port: Number(port)}).then((p) => console.log(`READY ${p}`));
// Never server.close(): it saves the page over the file it serves.
process.on("SIGTERM", () => process.exit(0));
