# NFS Retain Example

This example shows how you can specify a **reclaimPolicy** of **Retain** in order to retain the underlying
volume on an ibox and not have it removed by Kubernetes when you remove the PVC.

Our default is to have a **reclaimPolicy** of **Delete** on most of our examples, but for users that need 
to retain a ibox Volume for their application use case, this example shows what you can do to
support this use case.

The key things to notice in this example is in the StorageClass how you specify **Retain** as a reclaim policy.

Also, in the PV definition, you will see we are creating the PV manually in this case and specifying
a reclaim policy of **Retain** as well.  Then in the PVC definition, we reference the PV volume name
to access the manually created PV.

## Queries for PV Values
To work with this example, you have to query some volume related values using the ibox API.

These examples use the **jq** utility to format the json output nicely.

To look up the volume ID using a volume name, you can issue a command like this, where the volume name you are looking
for is **jeff-b3b8566819**:

```
curl https://ibox1234/api/rest/filesystems?name=jeff-b3b8566819 -u "someiboxuser:someiboxpsw" --insecure | jq
```

The output from the command is as follows, notice the 'id' field, that is what we will need for defining
the PV:
```json
{
  "result": [
    {
      "type": "MASTER",
      "depth": 0,
      "id": 94953794,
      "name": "jeff-b3b8566819",
      "created_at": 1695899901264,
      "updated_at": 1695899901264,
    }
  ]
}
```

Then with the volume ID, you can look up the exports using the API like this, where the volume ID is **94953794**:

```
curl https://ibox1234/api/rest/exports?filesystem_id=94953794 -u "someiboxuser:someiboxpsw" --insecure | jq
```


The output from the command is as follows, notice the 'id' field, that is what we will need for defining
the PV:

```json
{
  "result": [
    {
      "id": 450736,
      "export_path": "/jeff-b3b8566819",
      "inner_path": "/",
      "anonymous_uid": 65534,
      "anonymous_gid": 65534,
      "transport_protocols": "TCP",
      "max_read": 1048576,
      "max_write": 1048576,
      "pref_read": 262144,
      "pref_write": 262144,
      "pref_readdir": 65536,
      "privileged_port": false,
      "make_all_users_anonymous": false,
      "32bit_file_id": false,
      "created_at": 1695907221338,
      "updated_at": 1695907221338,
      "enabled": true,
      "snapdir_visible": false,
      "character_encoding": "UTF-8",
      "filesystem_id": 94953794,
      "tenant_id": 1,
      "permissions": [
        {
          "access": "RW",
          "client": "172.31.91.106",
          "no_root_squash": true
        }
      ]
    }
  ],
  "error": null,
  "metadata": {
    "ready": true,
    "number_of_objects": 1,
    "page_size": 50,
    "pages_total": 1,
    "page": 1
  }
}
```

## PV Definition 

The export ID returned from the query above is required for the PV definition.  Also the IP address of the NFS network space which is found in 
the ibox web console under NAS network space details is also required when defining the PV.  For example, in pv.yaml, you would specify
the following values:
```yaml
    volumeAttributes:
      exportID: "450736"
      ipAddress: "172.31.32.158"
``` 

The pv.yaml also requires you to specify the *volPathd* in the *volumeAttributes* section, this matches the volume name.  For
example:
```yaml
    volumeAttributes:
      volPathd: /jeff-b3b8566819
``` 

Note that you also will use the volume ID field when defining the volumeHandle field in the PV, for example
that value will look like the following with a volume ID of **94953794**:
```yaml
volumeHandle: 94953794$$nfs
```
