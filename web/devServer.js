var fs = require('fs');
var uri = require('url');
var WebpackDevServer = require("webpack-dev-server");
var webpackConfig = require('./webpack/webpack.dev.config.js');
var webpack = require('webpack');
var proxy = require('http-proxy').createProxyServer();
var changeProxyResponse = require('./devServerUtils');

// parse target URL
var argv = require('optimist')
    .usage('Usage: $0 -proxy [url]')
    .demand(['proxy'])
    .argv;

var urlObj = uri.parse(argv.proxy)

if (!urlObj.host) {
  console.error('invalid URL: ' + argv.proxy);
  return;
}

var PROXY_TARGET = urlObj.host;
var ROOT = '/web';
var PORT = '8080';
var WEBPACK_CLIENT_ENTRY = 'webpack-dev-server/client?https://0.0.0.0:' + PORT;
var WEBPACK_SRV_ENTRY = 'webpack/hot/only-dev-server';

for (var prop in webpackConfig.entry) {
  webpackConfig.entry[prop].unshift('react-hot-loader/patch');
  webpackConfig.entry[prop].unshift(WEBPACK_CLIENT_ENTRY, WEBPACK_SRV_ENTRY);
}

function getTargetOptions() {
  return {
    target: "https://"+PROXY_TARGET,
    secure: false,
    changeOrigin: true,
    xfwd: true
  }
}

var compiler = webpack(webpackConfig);

var server = new WebpackDevServer(compiler, {
  proxy:{
    '/web/grafana/*': getTargetOptions(),
    '/web/config.*': getTargetOptions(),
    '/pack/v1/*': getTargetOptions(),
    '/portalapi/*': getTargetOptions(),
    '/portal*': getTargetOptions(),
    '/proxy/*': getTargetOptions(),
    '/v1/*': getTargetOptions(),
    '/app/*': getTargetOptions(),
    '/sites/v1/*': getTargetOptions()
  },
  publicPath: ROOT +'/app',
  hot: true,
  disableHostCheck: true,
  https: true,
  inline: true,
  headers: { 'X-Custom-Header': 'yes' },
  //stats: { colors: true },
  stats: 'errors-only'
});

// tell webpack dev server to proxy below sockets requests to actual server
server.listeningApp.on('upgrade', function(req, socket) {
  //if (req.url.match('/v1/webapi/sites')) {
    console.log('proxying ws', req.url);
    proxy.ws(req, socket, {
      target: 'wss://' + PROXY_TARGET,
      secure: false
    });
  //}
});

var htmlToSend = fs.readFileSync(__dirname + "//dist//index.html", 'utf8')

// to enable Hot Module Reload we need to serve local index.html.
// since local index.html has no embeded TOKEN, we need to:
// 1) make a proxy request
// 2) modify proxy response by replacing server index.html with the local
// 3) insert embeded by server token into the local
server.app.use(changeProxyResponse(
    (req, res) => {
        // return true if you want to modify the response later
        var contentType = res.getHeader('Content-Type');
        if (contentType && contentType.startsWith('text/html')) {
            return true;
        }

        return false;
    },
    (req, res, body) => {
        // body is a Buffer with the current response; return Buffer or string with the modified response
        // can also return a Promise.
        var str = body.toString();
        res.set({
          'content-security-policy': ""
        })

        if (req.path.endsWith('/complete/')) {
          return body;
        }

        var htmlWithTokens = htmlToSend;
        htmlWithTokens = replaceToken(new RegExp(/<meta name="grv_csrf_token" .*\>/), str, htmlWithTokens);
        htmlWithTokens = replaceToken(new RegExp(/<meta name="grv_bearer_token" .*\>/), str, htmlWithTokens);
        return htmlWithTokens;
    }
));

function serveHTML() {
  return function (req, res) {
    proxy.web(req, res,  getTargetOptions());
  }
}

server.app.get(ROOT +'/*', serveHTML());
server.app.get(ROOT, serveHTML());

server.listen(PORT, "0.0.0.0", function() {
  console.log('Dev Server is up and running: https://localhost:' + PORT + '/web/');
});

function replaceToken(regex, takeFrom, insertTo){
  var value = takeFrom.match(regex);
  if(value){
    return insertTo.replace(regex, value[0]);
  }
  return insertTo;
}
