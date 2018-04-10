<template>
  <v-app :dark="dark">
    <v-navigation-drawer
      persistent
      :mini-variant="miniVariant"
      clipped="true"
      v-model="drawer"
      enable-resize-watcher
      fixed
      app
    >
      <v-list>
        <v-list-tile
          value="true"
          v-for="(item, i) in items"
          :key="i"
          :to="item.path == '#' ? '' : item.path"
        >
          <v-list-tile-action>
            <v-icon v-html="item.icon"></v-icon>
          </v-list-tile-action>
          <v-list-tile-content>
            <v-list-tile-title v-text="item.title"></v-list-tile-title>
          </v-list-tile-content>
        </v-list-tile>
      </v-list>
    </v-navigation-drawer>
    <v-toolbar
      app
      clipped-left="true"
    >
      <v-toolbar-side-icon @click.stop="drawer = !drawer"></v-toolbar-side-icon>
      <v-btn icon @click.stop="miniVariant = !miniVariant">
        <v-icon v-html="miniVariant ? 'chevron_right' : 'chevron_left'"></v-icon>
      </v-btn>
      <v-toolbar-title v-text="title"></v-toolbar-title>
    </v-toolbar>
    <v-content>
      <router-view/>
    </v-content>
  </v-app>
</template>

<script>
export default {
  name: "App",
  data() {
    return {
      drawer: true,
      items: [
        {
          icon: "people",
          title: "Timeline",
          path: "/timeline"
        },
        {
          icon: "public",
          title: "Network",
          path: "/network"
        },
        {
          icon: "portrait",
          title: "Profile",
          path: "/profile"
        },
        {
          icon: "mail_outline",
          title: "Messages",
          path: "/messages"
        },
        {
          icon: "photo_library",
          title: "Media",
          path: "/media"
        },
        {
          icon: "collections_bookmark",
          title: "Bookmarks",
          path: "/bookmarks"
        },
        {
          icon: "cloud",
          title: "Storage",
          path: "/storage"
        },
        {
          icon: "settings",
          title: "Settings",
          path: "/settings"
        }
      ],
      miniVariant: false,
      title: "Sprezz",
      dark: false
    };
  }
};
</script>
