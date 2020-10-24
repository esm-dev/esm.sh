# mmdb_china_ip_list

[README](README_en.md) | [中文说明](README.md)

![Daily Build](https://github.com/alecthw/mmdb_china_ip_list/workflows/Daily%20Build/badge.svg)  ![Release Build](https://github.com/alecthw/mmdb_china_ip_list/workflows/Release%20Build/badge.svg)

Overlap the Chinese IP list published by `china_ip_list` and `chunzhen CN` into the official community edition database of `MaxMind`.

This is also an example of generating MaxMind Database!

It's suitable for using in network offloading tools and compatible with MaxMind DB client!
It is more friendly to Chinese IP matching and distribution.

Automatically pull new MaxMind, china_ip_list and Chunzhen CN databases every week, and release a new Release version.


## Fixed download connection

| File | release branch | Aliyun |
| ------ | ------ | ------ |
| Country.mmdb | [link](https://raw.githubusercontent.com/alecthw/mmdb_china_ip_list/release/Country.mmdb) | [link](http://www.ideame.top/mmdb/Country.mmdb) |
| version | [link](https://raw.githubusercontent.com/alecthw/mmdb_china_ip_list/release/version) | [link](http://www.ideame.top/mmdb/version) |

## Introduction

 Is not very friendly to Chinese IP matching. So, there are many problems in actual using the `GeoLite2-Country` of [MaxMind](https://www.maxmind.com/en/home) in network tapping tools (such as Clash).

This project, on the basis of the MaxMind database, added [china_ip_list](https://raw.githubusercontent.com/17mon/china_ip_list/master/china_ip_list.txt) and [Pure CN Database](https://raw.githubusercontent .com/metowolf/iplist/master/data/country/CN.txt), making it more friendly to Chinese IP matching.

## How to use

Download the generated `china_ip_list.mmdb` from [Release](https://github.com/alecthw/mmdb_china_ip_list/releases).

The usage is the same as the official API of MaxMind, please refer to [Guide Document](http://maxmind.github.io/MaxMind-DB/).

### Using in OpenClash

Rename `china_ip_list.mmdb` to `Country.mmdb`, then replace `/etc/openclash/Country.mmdb`, and finally restart clash.

You can edit the `update url` in update script and enable auto update.

## Build mmdb

The `perl` environment is required. For the dependency and use of `MaxMind-DB-Writer-perl`, please refer to [Official Document](https://github.com/maxmind/MaxMind-DB-Writer-perl).

``` bash
# Download mmdb writer
git clone https://github.com/maxmind/MaxMind-DB-Writer-perl.git writer
cd writer

# Install dependencies
curl -LO http://xrl.us/cpanm
perl cpanm –installdeps .

# Build
./Build manifest
perl Build.PL
./Build install

# Return to parent directory
cd ..

# Clone this project
git clone https://github.com/alecthw/mmdb_china_ip_list.git
cd mmdb_china_ip_list

# Download GeoLite2-Country-CSV
curl -LR -o GeoLite2-Country-CSV.zip "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country-CSV&license_key=JvbzLLx7qBZT&suffix=zip"
unzip GeoLite2-Country-CSV.zip
rm -f GeoLite2-Country-CSV.zip
mv GeoLite2* mindmax

# Download china_ip_list
curl -LR -o china_ip_list.txt "https://raw.githubusercontent.com/17mon/china_ip_list/master/china_ip_list.txt"

# Download Chunzhen CN
curl -LR -o CN.txt "https://raw.githubusercontent.com/metowolf/iplist/master/data/special/china.txt"

# Generate mmdb
perl china_ip_list.pl
```
The generated file is`china_ip_list.mmdb`。

## MaxMind GeoIP Format

The official said little about the content of their own database.
It's the format that I found out  with debugging the source code. And then generated the database.

Examples of all fields are listed below for reference.

header
``` json
{
    "database_type": "GeoLite2-Country",
    "binary_format_major_version": 2,
    "build_epoch": 1589304057,
    "ip_version": 6,
    "languages": [
        "de",
        "en",
        "es",
        "fr",
        "ja",
        "pt-BR",
        "ru",
        "zh-CN"
    ],
    "description": {
        "en": "GeoLite2 Country database"
    },
    "record_size": 24,
    "node_count": 616946,
    "binary_format_minor_version": 0
}
```

network-field
``` json
{
    "continent": {
        "code": "AS",
        "names": {
            "de": "Asien",
            "ru": "Азия",
            "pt-BR": "Ásia",
            "ja": "アジア",
            "en": "Asia",
            "fr": "Asie",
            "zh-CN": "亚洲",
            "es": "Asia"
        },
        "geoname_id": 6255147
    },
    "country": {
        "names": {
            "de": "China",
            "ru": "Китай",
            "pt-BR": "China",
            "ja": "中国",
            "en": "China",
            "fr": "Chine",
            "zh-CN": "中国",
            "es": "China"
        },
        "iso_code": "CN",
        "geoname_id": 1814991,
        "is_in_european_union": false,
    },
    "registered_country": {
        "names": {
            "de": "China",
            "ru": "Китай",
            "pt-BR": "China",
            "ja": "中国",
            "en": "China",
            "fr": "Chine",
            "zh-CN": "中国",
            "es": "China"
        },
        "iso_code": "CN",
        "geoname_id": 1814991
    },
    "represented_country": {
        "names": {
            "de": "China",
            "ru": "Китай",
            "pt-BR": "China",
            "ja": "中国",
            "en": "China",
            "fr": "Chine",
            "zh-CN": "中国",
            "es": "China"
        },
        "iso_code": "CN",
        "geoname_id": 1814991
    },
    "traits": {
        "is_anonymous_proxy": true,
        "is_satellite_provider": true
    }
}
```

## Thanks

- [GeoIP MaxMind DB 生成指南](https://blog.csdn.net/openex/article/details/53487465)

- [GeoLite Mirror | Sukka](https://geolite.clash.dev/)

- [使用 GeoLite 实现IP精准定位(Java实现)](https://www.jianshu.com/p/1b1a018ae729)

- [Loyalsoldier提供的GeoLite2-Country-CSV下载链接](https://github.com/Loyalsoldier/v2ray-rules-dat)

## Last

It's not easy to find out. I hope to leave a name for citation integration or secondary release...
