Container instances are launched on-demand in response to requests from a Worker, and can be put to sleep after a conifigurable timeout.

Containers can run a wide variety of languages and tools, use more memory and CPU than a Worker, and run for longer periods of time.

"Just let me deploy it!"
If you want to skip ahead and deploy your Container-enabled Worker, you can click the "Deploy to Cloudflare" button or run the following command from your terminal:

$ npm create cloudflare@latest -- --template=cloudflare/templates/containers-template

Prerequisites
You must have the following permissions to deploy a Container-enabled Worker:

"Containers: Edit"
"Workers Scripts: Edit"

Building a Container Image
Images on Cloudflare are defined with a Dockerfile or referenced via a URL.

Container code can be is written alongside Worker code. Both are then deployed together. To do this, you include a standard Dockerfile in your Worker repository alongside any necessary source code.

In this example, we'll include a simple Golang server that responds to requests on port 8080 using the MESSAGE environment variable that is set in the Worker:

```
func handler(w http.ResponseWriter, r *http.Request) {
  message := os.Getenv("MESSAGE")
  instanceId := os.Getenv("CLOUDFLARE_DEPLOYMENT_ID")

  fmt.Fprintf(w, "Hi, I'm a container and this is my message: %s, and my instance ID is: %s", message, instanceId)
}
```

We'll be able to interact with this server through Workers, including proxying external user requests to it.

Wrangler Config
Container Configuration
When defining your Worker, you instruct it to use a container that is built with the local Dockerfile using the new "containers" field in wrangler config:

```
{
  "containers": [
    {
      "class_name": "MyContainer",
      "image": "./Dockerfile",
      "max_instances": 5,
    }
  ]
}
```

This config contains:

- `image` pointing to a Dockerfile or URL with a container image
- `class_name` the name of the Container class you define in Worker code
- `max_instances` the maximum number of simultaneously running container instances

Durable Object Configuration
In addition to the Container config, you need Durable Object configuration. This is because all Container are access via a Durable Object.

If you don't know what a Durable Object is, that's okay! Just think of each Durable Object as a single global instance that can store data and be easily routed to by ID. These are used to find and manage each Container instance.

Here's how you configure one. Note that "class_name" matches your container class_name:

```
{
  "durable_objects": {
      "bindings": [
        {
          "class_name": "MyContainer",
          "name": "MY_CONTAINER"
        }
      ]
    }
    "migrations": [
      {
        "new_sqlite_classes": [
          "MyContainer"
        ],
        "tag": "v1"
      }
    ]
}
```

Defining a Container Class
Basic Configuration
You'll define your Container in your Worker using the Container class from @cloudflare/containers:

```
import { Container } from "@cloudflare/containers"

export class MyContainer extends Container {
  defaultPort = 8080;
  sleepAfter = '5m';
  envVars = {
    MESSAGE: 'I was passed in via the container class!',
  };
}
```

This defines basic configuration for the container:

- `defaultPort` sets the port that fetch uses to communicate with the container.
- `sleepAfter` sets the timeout for the container to sleep after if it has been idle.
- `envVars` sets environment variables that are used by the container.

You can also include onStart, onStop, and onError hooks that run when the container starts, stops, or errors.
There is a lot more the Container class can do, such as manually starting and stopping your instance, running hooks on status changes, and storing state for the container instance. See the Container class documentation for more details: https://developers.cloudflare.com/containers/container-package

Note
The Container class is actually a Durable Objects under the hood, but you don't have to be familiar with Durable Objects to use Containers.

Routing to a Container from your Worker
Getting unique Container Instances
Container instances are accessed via Workers.

To spin up a Container instance, call get on its binding with an ID:

```
const containerInstance = env.MY_CONTAINER.get(id);
```

Each unique ID passed to get will result in a new Container instance launching. Multiple Worker requests calling to the same ID will route requests to the same Container instance.

Making container requests
Once you have a container instance, you can make requests to it using fetch, which will automatically make requests to the defaultPort. You can pass in the request value from your Worker's fetch handler to proxy requests to the Worker. This includes WebSocket requests.

```
return containerInstance.fetch(request);
```

You can also define custom RPC calls on your Container class and call those.

Example: Routing to the same container instances
If you want to route all requests to a single instance, pass in the same ID every time:

```
const id = env.MY_CONTAINER.idFromName('singleton');
const containerInstance = env.MY_CONTAINER.get(id);
return await containerInstance.fetch(request);
```

Example: Routing to a unique container instances per request
If you want different instances, pass in different IDs. For instance, you could get an ID from a request header:

```
const sessionId = request.headers.get("session-id")
const id = env.MY_CONTAINER.idFromName(sessionId);
const containerInstance = env.MY_CONTAINER.get(id);
return await containerInstance.fetch(request);
```

Example: Load balancing across container instances
If you want to load balance across a few containers, you can use the getRandom helper from @cloudflare/containers to balance requests over several instances (in this case three):

```
import { getRandom } from "@cloudflare/containers"

const containerInstance = await getRandom(env.MY_CONTAINER, 3);
return containerInstance.fetch(request);
```

Note
The getRandom helper is a temporary solution until support for utilization-aware autoscaling and latency-aware load balancing is added to Cloudflare Containers. We plan to add this in the coming months.

Deploying your Container-Enabled Worker
Run the following command to deploy your Worker and Container:

```
npx wrangler deploy
```

When you run this command, several things happen:

Wrangler builds your image locally.
Wrangler pushes your image to the Cloudflare Registry, which is automatically integrated with your account.
Your image is automatically distributed across Cloudflare's Network and prepped for fast boots.
Wrangler deploys your Worker.
When we will build and push a container images, by default, this process uses Docker. You must have Docker or a Docker-compatible CLI tool running locally when you run wrangler deploy.

After you deploy your Worker for the first time, you will need to wait a few minutes until it is ready to receive requests. During this time, requests are sent to the Worker, but calls to the Container will error.

Note
The build and push usually take the longest on the first deploy. Future deploys will go faster by reusing cached image layers.

Test your Container
Making requests to your Worker
Open the URL for your Worker. It should look something like https://hello-containers.YOUR_ACCOUNT_NAME.workers.dev.

If you make requests to the paths /container/1 or /container/2, these requests are routed to specific containers. Each different path after "/container/" routes to a unique container.

If you make requests to /lb, you will load balance requests to one of 3 containers chosen at random.

You can confirm this behavior by reading the output of each request.

That's It!
Now you know the basics of Containers and have a simple Container-enabled Worker.

Feel free to change your Container or Worker code and redeploy with $ npx wrangler deploy

See the Container docs for more info.


# Another example

---
title: Static Frontend, Container Backend
description: A simple frontend app with a containerized backend
image: https://developers.cloudflare.com/dev-products-preview.png
---

[Skip to content](#%5Ftop) 

Was this helpful?

YesNo

[ Edit page ](https://github.com/cloudflare/cloudflare-docs/edit/production/src/content/docs/containers/examples/container-backend.mdx) [ Report issue ](https://github.com/cloudflare/cloudflare-docs/issues/new/choose) 

Copy page

# Static Frontend, Container Backend

**Last reviewed:**  9 months ago 

A simple frontend app with a containerized backend

A common pattern is to serve a static frontend application (e.g., React, Vue, Svelte) using Static Assets, then pass backend requests to a containerized backend application.

In this example, we'll show an example using a simple `index.html` file served as a static asset, but you can select from one of many frontend frameworks. See our [Workers framework examples](https://developers.cloudflare.com/workers/framework-guides/web-apps/) for more information.

For a full example, see the [Static Frontend + Container Backend Template ↗](https://github.com/mikenomitch/static-frontend-container-backend).

## Configure Static Assets and a Container

* [  wrangler.jsonc ](#tab-panel-4017)
* [  wrangler.toml ](#tab-panel-4018)

JSONC

```

{

  "name": "cron-container",

  "main": "src/index.ts",

  "assets": {

    "directory": "./dist",

    "binding": "ASSETS"

  },

  "containers": [

    {

      "class_name": "Backend",

      "image": "./Dockerfile",

      "max_instances": 3

    }

  ],

  "durable_objects": {

    "bindings": [

      {

        "class_name": "Backend",

        "name": "BACKEND"

      }

    ]

  },

  "migrations": [

    {

      "new_sqlite_classes": [

        "Backend"

      ],

      "tag": "v1"

    }

  ]

}


```

TOML

```

name = "cron-container"

main = "src/index.ts"


[assets]

directory = "./dist"

binding = "ASSETS"


[[containers]]

class_name = "Backend"

image = "./Dockerfile"

max_instances = 3


[[durable_objects.bindings]]

class_name = "Backend"

name = "BACKEND"


[[migrations]]

new_sqlite_classes = [ "Backend" ]

tag = "v1"


```

## Add a simple index.html file to serve

Create a simple `index.html` file in the `./dist` directory.

index.html

```

<!DOCTYPE html>

<html lang="en">


<head>

  <meta charset="UTF-8">

  <meta name="viewport" content="width=device-width, initial-scale=1.0">

  <title>Widgets</title>

  <script defer src="https://cdnjs.cloudflare.com/ajax/libs/alpinejs/3.13.3/cdn.min.js"></script>

</head>


<body>

  <div x-data="widgets()" x-init="fetchWidgets()">

    <h1>Widgets</h1>

    <div x-show="loading">Loading...</div>

    <div x-show="error" x-text="error" style="color: red;"></div>

    <ul x-show="!loading && !error">

      <template x-for="widget in widgets" :key="widget.id">

        <li>

          <span x-text="widget.name"></span> - (ID: <span x-text="widget.id"></span>)

        </li>

      </template>

    </ul>


    <div x-show="!loading && !error && widgets.length === 0">

      No widgets found.

    </div>


  </div>


  <script>

    function widgets() {

      return {

        widgets: [],

        loading: false,

        error: null,


        async fetchWidgets() {

          this.loading = true;

          this.error = null;


          try {

            const response = await fetch('/api/widgets');

            if (!response.ok) {

              throw new Error(`HTTP ${response.status}: ${response.statusText}`);

            }

            this.widgets = await response.json();

          } catch (err) {

            this.error = err.message;

          } finally {

            this.loading = false;

          }

        }

      }

    }

  </script>


</body>


</html>


```

In this example, we are using [Alpine.js ↗](https://alpinejs.dev/) to fetch a list of widgets from `/api/widgets`.

This is meant to be a very simple example, but you can get significantly more complex. See [examples of Workers integrating with frontend frameworks](https://developers.cloudflare.com/workers/framework-guides/web-apps/) for more information.

## Define a Worker

Your Worker needs to be able to both serve static assets and route requests to the containerized backend.

In this case, we will pass requests to one of three container instances if the route starts with `/api`, and all other requests will be served as static assets.

JavaScript

```

import { Container, getRandom } from "@cloudflare/containers";


const INSTANCE_COUNT = 3;


class Backend extends Container {

  defaultPort = 8080; // pass requests to port 8080 in the container

  sleepAfter = "2h"; // only sleep a container if it hasn't gotten requests in 2 hours

}


export default {

  async fetch(request, env) {

    const url = new URL(request.url);

    if (url.pathname.startsWith("/api")) {

      // note: "getRandom" to be replaced with latency-aware routing in the near future

      const containerInstance = await getRandom(env.BACKEND, INSTANCE_COUNT);

      return containerInstance.fetch(request);

    }


    return env.ASSETS.fetch(request);

  },

};


```

Note

This example uses the `getRandom` function, which is a temporary helper that will randomly select of of N instances of a Container to route requests to.

In the future, we will provide improved latency-aware load balancing and autoscaling.

This will make scaling stateless instances simple and routing more efficient. See the[autoscaling documentation](https://developers.cloudflare.com/containers/platform-details/scaling-and-routing) for more details.

## Define a backend container

Your container should be able to handle requests to `/api/widgets`.

In this case, we'll use a simple Golang backend that returns a hard-coded list of widgets.

server.go

```

package main


import (

  "encoding/json"

  "log"

  "net/http"

)


func handler(w http.ResponseWriter, r \*http.Request) {

  widgets := []map[string]interface{}{

    {"id": 1, "name": "Widget A"},

    {"id": 2, "name": "Sprocket B"},

    {"id": 3, "name": "Gear C"},

  }


  w.Header().Set("Content-Type", "application/json")

  w.Header().Set("Access-Control-Allow-Origin", "*")

  json.NewEncoder(w).Encode(widgets)


}


func main() {

  http.HandleFunc("/api/widgets", handler)

  log.Fatal(http.ListenAndServe(":8080", nil))

}


```

```json
{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"item":{"@id":"/directory/","name":"Directory"}},{"@type":"ListItem","position":2,"item":{"@id":"/containers/","name":"Containers"}},{"@type":"ListItem","position":3,"item":{"@id":"/containers/examples/","name":"Examples"}},{"@type":"ListItem","position":4,"item":{"@id":"/containers/examples/container-backend/","name":"Static Frontend, Container Backend"}}]}
```
