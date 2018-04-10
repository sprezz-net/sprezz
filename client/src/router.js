import Vue from "vue";
import Router from "vue-router";
import Home from "./views/Home.vue";
import Timeline from "./views/Timeline.vue";
import Network from "./views/Network.vue";
import Profile from "./views/Profile.vue";
import Messages from "./views/Messages.vue";
import Media from "./views/Media.vue";
import Storage from "./views/Storage.vue";
import Bookmarks from "./views/Bookmarks.vue";
import Settings from "./views/Settings.vue";

Vue.use(Router);

export default new Router({
  mode: "history",
  routes: [
    {
      path: "/",
      name: "home",
      component: Home
    },
    {
      path: "/timeline",
      name: "timeline",
      component: Timeline
    },
    {
      path: "/network",
      name: "network",
      component: Network
    },
    {
      path: "/profile",
      name: "profile",
      component: Profile
    },
    {
      path: "/messages",
      name: "messages",
      component: Messages
    },
    {
      path: "/media",
      name: "media",
      component: Media
    },
    {
      path: "/bookmarks",
      name: "bookmarks",
      component: Bookmarks
    },
    {
      path: "/storage",
      name: "storage",
      component: Storage
    },
    {
      path: "/settings",
      name: "settings",
      component: Settings
    }
  ]
});
