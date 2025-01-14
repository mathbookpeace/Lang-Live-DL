# Lang-Live-DL

## Installation

1. Install Go:
   ```shell
   brew install go
   ```
1. Install ffmpeg:
   ```shell
   brew install ffmpeg
   ```
1. Run Lang-Live-DL
   ```shell
   go run lang_live_dl.go
   ```

## Configs

Here are the configurations you can set in the configs.json file:

```C++
{
   "default_configs": {
      // If set to true, the program will delete the stream record when the stream ends.
      // This option is useful if you only want to receive a notification when the stream starts.
      "check_only": false,

      // This variable sets the default path for all members. If not set, the default path will be "data".
      "default_folder": "path_to_a_folder"
   },
   "members": [
      {
         // 浪live ID of the member
         "id": 3619520,

         // Name of the member
         "name": "李佳俐",

         // Whether to receive a notification when the stream starts
         "enable_notify": true,

         // The folder where the stream record for this member will be placed.
         // If you don't set this value, the file will be put in the default folder.
         "folder": "path_to_a_folder",

         // The default output file name is "name.datetime".
         // If you set the prefix for a member, the output file name for this member will be "prefix" + "datetime".
         "prefix": "file name prefix"
      },
      {
         "id": 3651219,
         "name": "李采潔",
         "enable_notify": true
      }
   ]
}
```