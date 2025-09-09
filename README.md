Dave Peckham's Solution to "Challenge - AIS viewer"
=====

This was pretty straightforward. I haven't used React or React Native in some time, so it took a little research to refamiliarize myself. I prefer to use Docker for all my development to keep my local machine clean, so I did that. The ingestion task and the server for the client seemed separate to me.

Dave's Notes and TODO Items
-----
1. AIS stream reports heading 511 which is "no data". I ignore that for now. Could use COG if available. Or have a sensible default. Typical heading vs COG for slow speed discussion here :-)
1. In the event the network fails, I am not removing any targets. An advanced solution might have the client "add" all new targets from the websocket response to a local, in-memory map, then periodically remove items from the map on the client side. This would have additional UI benefits as well.
1. For demo purposes, I am only ingesting AIS data from New York to Boston. Other approaches would be to add Bounding Boxes for each client request, but long-term, a system like this would be looking at the whole world, so that optimization seems unneccesary.
1. The client app re-subscribes each time the bounds of the display change. Again, some local cache could also be used to improve performance and continuity when moving back to a previously loaded location, but that's not strictly required here. Same applies for rotating the map, zooming in and out again.
1. I suspect that the bounding box concept as a pure North-up rectangle is not adequte when the map is rotated to a non-90 degree orientation. Then the bounding box should really be a polygon or perhaps use max and min of lat/lon corners to capture everything.
1. For a production project, I'd take the time to extract common code from the two Golang projects into a library. DRY.
1. The instructions said to only /display/ at a zoom level. Ideally I'd also stop retrieving data when not displaying it. Future optimization possibility. Trying to resist over-engineering :-) 

Startup
-----
```sh
docker compose up
```
and for the client:
```sh
cd AisViewer
npm i
npx react-native run-ios
```

Challenge - AIS viewer
=====

>The automatic identification system, or AIS, transmits a ship’s position so that other ships are aware of its position. The International Maritime Organization and other management bodies require large ships, including many commercial fishing vessels, to broadcast their position with AIS in order to avoid collisions. Each year, more than 400,000 AIS devices broadcast vessel location, identity, course, and speed information. Ground stations and satellites pick up this information, making vessels trackable even in the most remote areas of the ocean.[1]

Task
-----

Build an end-to-end AIS system that ingests AIS data from a free provider and displays the data in near real-time on a map.


Details
-----

### Backend

1. ~~Ingest AIS data from aisstream.io[2]~~
2. ~~For simplicity only handle AIS message PositionReport[3]~~
3. ~~Keep in mind that there could be up to 30k vessels for full world coverage, and most of them are relatively fresh (updated within 10 minutes)~~
4. ~~Use a database with a geospatial extension (PostGIS, SpatiaLite, MongoDB…) to store the vessel data~~
5. ~~Vessel data should persist and fresh data should instantly be available for the front end. Backend reboot should keep previously processed data.~~
6. ~~You are free to choose how to expose the data from the database to the front end, but keep in mind the possible amount of data~~

### Frontend

1. ~~Use Mapbox Maps SDK for React Native[4] for displaying the map~~
2. ~~Show vessels on the map~~
    - ~~Make sure that the course of the vessel is clearly visible via the direction of the marker~~
3. ~~Display vessels only when map zoom level 12 or more to set a boundary for client-side performance~~
4. ~~Displayed vessels should be near real-time - max 10 seconds delay from receiving it on the backend to being added/updated on the map~~
5. ~~Displayed vessels should be fresh - don’t show vessels that haven’t been updated in the past 2 minutes~~


Notes
-----

We value simplicity and elegance as opposed to over-engineering.

Consider that the client side only needs to retrieve data for the current map view, which given the Frontend point 3 is only a small subset of the full dataset. Retrieving the full set every 10 seconds would not be feasible.

The system should support many simultaneous map viewers.


Deliverable
-----

1. Video demonstrating the system
2. Source code.


References
-----

- [1] https://globalfishingwatch.org/faqs/what-is-ais
- [2] https://aisstream.io/documentation
- [3] https://aisstream.io/documentation#PositionReport
- [4] https://github.com/rnmapbox/maps
